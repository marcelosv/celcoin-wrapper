package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Config via env
// SERVER_ADDR=:8443
// TLS_CERT=certs/server.crt
// TLS_KEY=certs/server.key
// MTLS_ENABLED=true|false
// MTLS_CA_CERT=certs/ca.crt (optional if MTLS_ENABLED=true)
// TESTDATA_DIR=testdata
// WEBHOOK_TARGET=http://localhost:8080/v1/webhooks/celcoin
// WEBHOOK_BEARER=optional-bearer
// WEBHOOK_HMAC_SECRET=optional-hmac-secret

func getenv(key, def string) string {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	return v
}

// --- Pix Out: create payment ---
func pixOutCreateHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    idem := r.Header.Get("Idempotency-Key")
    var body struct {
        AccountProviderId string  `json:"accountProviderId"`
        Amount            float64 `json:"amount"`
        Currency          string  `json:"currency"`
        Description       string  `json:"description"`
        PixKey            string  `json:"pixKey"`
        Beneficiary       map[string]any `json:"beneficiary"`
    }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
        badRequest(w, "invalid json")
        return
    }
    if body.Currency == "" {
        body.Currency = "BRL"
    }
    now := time.Now().UTC()
    pid := genID("pay")
    p := &Payment{
        ID:        pid,
        Status:    "PROCESSING",
        Amount:    body.Amount,
        Currency:  body.Currency,
        EndToEndId: genID("E2E"),
        TxId:      genID("TX"),
        Nsu:       genID("NSU"),
        CreatedAt: now,
        UpdatedAt: now,
    }
    p = pstore.Create(idem, p)
    w.Header().Set("Location", "/baas/v2/pix/out/payments/"+p.ID)
    mustJSON(w, http.StatusCreated, p)
}

// --- Pix Out: payment status by ID ---
func pixOutGetHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodGet {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
    if len(segments) < 6 { // baas v2 pix out payments {id}
        badRequest(w, "invalid path")
        return
    }
    id := segments[5]
    p, ok := pstore.Get(id)
    if !ok {
        mustJSON(w, http.StatusNotFound, map[string]any{"error": "not found"})
        return
    }
    mustJSON(w, http.StatusOK, p)
}

// --- Pix Out: QR Code parse ---
func qrParseHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    var body struct{ EMV string `json:"emv"` }
    if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.EMV == "" {
        badRequest(w, "invalid json or missing emv")
        return
    }
    // Mock parse output deterministically using lengths
    resp := map[string]any{
        "txId":   genID("TX"),
        "merchant": "Mock Merchant",
        "amount":  123.45,
        "city":    "Sao Paulo",
        "key":     "mock-pix-key",
        "e2eId":   genID("E2E"),
    }
    mustJSON(w, http.StatusOK, resp)
}

// --- Pix ITP: create consent ---
func itpConsentsHandler(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        w.WriteHeader(http.StatusMethodNotAllowed)
        return
    }
    // We accept any JSON body for the mock
    _, _ = io.ReadAll(r.Body)
    resp := map[string]any{
        "consentId": genID("consent"),
        "status":    "AWAITING_AUTHORIZATION",
        "expiresAt": time.Now().Add(10 * time.Minute).UTC().Format(time.RFC3339),
    }
    mustJSON(w, http.StatusCreated, resp)
}

func mustJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func badRequest(w http.ResponseWriter, msg string) {
	mustJSON(w, http.StatusBadRequest, map[string]any{"error": msg})
}

func unauthorized(w http.ResponseWriter, msg string) {
	mustJSON(w, http.StatusUnauthorized, map[string]any{"error": msg})
}

func getBearer(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return ""
	}
	return strings.TrimSpace(auth[7:])
}

// --- testdata helpers ---

type StatusData struct {
	Items    []map[string]any `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
}

type StatementsData struct {
	AccountID      string  `json:"accountId"`
	OpeningBalance float64 `json:"openingBalance"`
	ClosingBalance float64 `json:"closingBalance"`
	Currency       string  `json:"currency"`
}

type Payment struct {
	ID        string                 `json:"id"`
	Status    string                 `json:"status"`
	StatusReason string              `json:"statusReason,omitempty"`
	Amount    float64                `json:"amount"`
	Currency  string                 `json:"currency"`
	EndToEndId string                `json:"endToEndId"`
	TxId      string                 `json:"txId"`
	Nsu       string                 `json:"nsu"`
	Receipt   map[string]any         `json:"receipt,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

type PaymentStore struct {
	mu   sync.Mutex
	byID map[string]*Payment
	byKey map[string]*Payment // Idempotency-Key
}

var pstore = &PaymentStore{byID: map[string]*Payment{}, byKey: map[string]*Payment{}}

func genID(prefix string) string {
	// simple deterministic-ish id based on time
	return fmt.Sprintf("%s_%x", prefix, time.Now().UnixNano())
}

func (s *PaymentStore) Create(idemKey string, p *Payment) *Payment {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idemKey != "" {
		if ex, ok := s.byKey[idemKey]; ok {
			return ex
		}
	}
	s.byID[p.ID] = p
	if idemKey != "" {
		s.byKey[idemKey] = p
	}
	return p
}

func (s *PaymentStore) Get(id string) (*Payment, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.byID[id]
	if !ok {
		return nil, false
	}
	// simple status progression: after 2s, go to SUCCESS
	if p.Status == "PROCESSING" && time.Since(p.CreatedAt) > 2*time.Second {
		p.Status = "SUCCESS"
		p.StatusReason = ""
		p.UpdatedAt = time.Now().UTC()
		if p.Receipt == nil {
			p.Receipt = map[string]any{"url": "https://example.test/receipt/" + p.ID}
		}
	}
	return p, true
}

func loadJSON[T any](path string, dst *T) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func filterStatus(data StatusData, q map[string]string) StatusData {
	if len(q) == 0 {
		return data
	}
	var filtered []map[string]any
	for _, it := range data.Items {
		match := true
		for k, v := range q {
			if v == "" { // ignore empty
				continue
			}
			if got, ok := it[k]; ok {
				if fmt.Sprint(got) != v {
					match = false
					break
				}
			} else {
				match = false
				break
			}
		}
		if match {
			filtered = append(filtered, it)
		}
	}
	return StatusData{Items: filtered, Page: 1, PageSize: len(filtered)}
}

// --- handlers ---

func tokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	mustJSON(w, http.StatusOK, map[string]any{
		"access_token": "mock-token",
		"token_type":   "Bearer",
		"expires_in":   2400,
	})
}

// OAuth2 client_credentials via form-urlencoded (spec alternative)
func oauthTokenHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		badRequest(w, "invalid form")
		return
	}
	// Optionally validate r.Form.Get("grant_type"), client_id, client_secret, audience
	mustJSON(w, http.StatusOK, map[string]any{
		"access_token": "mock-token",
		"token_type":   "Bearer",
		"expires_in":   2400,
	})
}

func requireBearer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if getBearer(r) == "" {
			unauthorized(w, "missing bearer token")
			return
		}
		next.ServeHTTP(w, r)
	}
}

func statusHandler(testdataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var data StatusData
		path := filepath.Join(testdataDir, "status_responses.json")
		if err := loadJSON(path, &data); err != nil {
			log.Printf("error reading %s: %v", path, err)
			mustJSON(w, http.StatusInternalServerError, map[string]any{"error": "failed to load testdata"})
			return
		}

		q := map[string]string{
			"id":           r.URL.Query().Get("id"),
			"endToEndId":   r.URL.Query().Get("endToEndId"),
			"movementType": r.URL.Query().Get("movementType"),
			"clientCode":   r.URL.Query().Get("clientCode"),
		}
		res := filterStatus(data, q)
		mustJSON(w, http.StatusOK, res)
	}
}

func statementsHandler(testdataDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		segments := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(segments) < 5 { // baas v2 accounts {accountId} statements
			badRequest(w, "invalid path")
			return
		}
		accountID := segments[3]
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		_ = from
		_ = to

		var data StatementsData
		path := filepath.Join(testdataDir, "statements_responses.json")
		if err := loadJSON(path, &data); err != nil {
			// default simple fallback
			data = StatementsData{AccountID: accountID, OpeningBalance: 1000.0, ClosingBalance: 1250.5, Currency: "BRL"}
		}
		// override account id
		data.AccountID = accountID
		mustJSON(w, http.StatusOK, data)
	}
}

// Admin endpoint to fire webhooks to the local service
func fireWebhookHandler(target string, bearer string, hmacSecret string) http.HandlerFunc {
	client := &http.Client{Timeout: 10 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			badRequest(w, "invalid body")
			return
		}
		req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(string(body)))
		if err != nil {
			mustJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		req.Header.Set("Content-Type", "application/json")
		// set headers
		var event struct{ Type string `json:"type"` }
		_ = json.Unmarshal(body, &event)
		if event.Type != "" {
			req.Header.Set("X-Event-Type", event.Type)
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		if hmacSecret != "" {
			sig := computeHMAC(body, []byte(hmacSecret))
			req.Header.Set("X-Signature", sig)
		}
		resp, err := client.Do(req)
		if err != nil {
			mustJSON(w, http.StatusBadGateway, map[string]any{"error": err.Error()})
			return
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		mustJSON(w, http.StatusOK, map[string]any{
			"target":       target,
			"status_code":  resp.StatusCode,
			"responseBody": string(respBody),
		})
	}
}

func computeHMAC(data, secret []byte) string {
	h := hmac.New(sha256.New, secret)
	h.Write(data)
	return "sha256=" + hex.EncodeToString(h.Sum(nil))
}

func buildMux(testdataDir string, webhookTarget, webhookBearer, webhookHMAC string) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/v5/token", tokenHandler)
	mux.HandleFunc("/oauth/token", oauthTokenHandler)
	mux.HandleFunc("/baas/v2/status", requireBearer(statusHandler(testdataDir)))
	mux.HandleFunc("/baas/v2/accounts/", requireBearer(statementsHandler(testdataDir)))
	mux.HandleFunc("/baas/v2/pix/out/payments", requireBearer(pixOutCreateHandler))
	mux.HandleFunc("/baas/v2/pix/out/payments/", requireBearer(pixOutGetHandler))
	mux.HandleFunc("/baas/v2/pix/out/qrcode/parse", requireBearer(qrParseHandler))
	mux.HandleFunc("/baas/v2/pix/itp/consents", requireBearer(itpConsentsHandler))
	mux.HandleFunc("/_admin/fire-webhook", fireWebhookHandler(webhookTarget, webhookBearer, webhookHMAC))

	// Optional webhook register route
	mux.HandleFunc("/dda-servicewebhook-webservice/v1/webhook/register", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		mustJSON(w, http.StatusOK, map[string]any{"status": "registered"})
	})

	// health
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	return mux
}

func loadTLSConfig() (*tls.Config, error) {
	mtlsEnabled := strings.EqualFold(getenv("MTLS_ENABLED", "false"), "true")
	cfg := &tls.Config{}
	if mtlsEnabled {
		caPath := getenv("MTLS_CA_CERT", "")
		if caPath == "" {
			return nil, errors.New("MTLS_ENABLED=true but MTLS_CA_CERT not set")
		}
		caPEM, err := os.ReadFile(caPath)
		if err != nil {
			return nil, fmt.Errorf("read CA: %w", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("failed to append CA cert")
		}
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
		cfg.ClientCAs = pool
	}
	return cfg, nil
}

func main() {
	addr := getenv("SERVER_ADDR", ":8443")
	cert := getenv("TLS_CERT", "certs/server.crt")
	key := getenv("TLS_KEY", "certs/server.key")
	testdataDir := getenv("TESTDATA_DIR", "testdata")
	webhookTarget := getenv("WEBHOOK_TARGET", "http://localhost:8080/v1/webhooks/celcoin")
	webhookBearer := getenv("WEBHOOK_BEARER", "")
	webhookHMAC := getenv("WEBHOOK_HMAC_SECRET", "")

	mux := buildMux(testdataDir, webhookTarget, webhookBearer, webhookHMAC)

	tlsCfg, err := loadTLSConfig()
	if err != nil {
		log.Fatalf("TLS config error: %v", err)
	}

	srv := &http.Server{Addr: addr, Handler: logMiddleware(mux), TLSConfig: tlsCfg}
	log.Printf("celcoin_wrapper listening on https://localhost%v", addr)
	log.Printf("mTLS: %v | testdata: %s", strings.EqualFold(getenv("MTLS_ENABLED", "false"), "true"), testdataDir)
	if err := srv.ListenAndServeTLS(cert, key); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: 200}
		next.ServeHTTP(rec, r)
		log.Printf("%s %s %d %s", r.Method, r.URL.String(), rec.status, time.Since(start))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}
