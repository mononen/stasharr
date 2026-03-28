package myjdownloader

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	apiBase = "https://api.jdownloader.org"
	// MD5 of empty string — the canonical appkey used by the reference
	// Python myjdapi client and several other working implementations.
	appKey = "d41d8cd98f00b204e9800998ecf8427e"
)

// Client is a MyJDownloader API client. It manages session state and
// transparently re-connects when a session expires.
type Client struct {
	email      string
	password   string
	DeviceName string
	httpClient *http.Client

	mu                  sync.Mutex
	sessionToken        string
	deviceID            string
	serverEncryptionKey []byte // 32 bytes
	deviceEncryptionKey []byte // 32 bytes
	loginSecret         []byte // 32 bytes
	deviceSecret        []byte // 32 bytes
	rid                 atomic.Int64
}

// New returns a new Client. Connect must be called before any device calls.
func New(email, password, deviceName string) *Client {
	ls := deriveSecret(email, password, "server")
	ds := deriveSecret(email, password, "device")
	c := &Client{
		email:        email,
		password:     password,
		DeviceName:   deviceName,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
		loginSecret:  ls,
		deviceSecret: ds,
	}
	// Initialise rid to the current Unix millisecond timestamp.
	// The MyJDownloader API expects rid to be a large, monotonically
	// increasing value (most reference clients use ms timestamps).
	c.rid.Store(time.Now().UnixMilli())
	return c
}

// Package represents a JDownloader download package.
type Package struct {
	Name        string `json:"name"`
	Status      string `json:"status"`
	Finished    bool   `json:"finished"`
	Running     bool   `json:"running"`
	BytesTotal  int64  `json:"bytesTotal"`
	BytesLoaded int64  `json:"bytesLoaded"`
}

// Device represents a JDownloader device registered in MyJDownloader.
type Device struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// Connect authenticates with MyJDownloader and resolves the configured device ID.
// It is safe to call Connect multiple times; it re-authenticates each time.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	rid := c.rid.Add(1)
	// URL-encode the email (@→%40) to match the reference myjdapi implementation.
	// No apiVer in GET requests — that parameter is POST-only.
	// The HMAC must be computed over the exact URL-encoded path sent to the server.
	queryString := fmt.Sprintf("/my/connect?email=%s&appkey=%s&rid=%d",
		url.QueryEscape(c.email), appKey, rid)
	sig := signHMAC(queryString, c.loginSecret)
	queryString += "&signature=" + sig

	log.Printf("[myjdownloader] connect URL: %s%s", apiBase, queryString)

	resp, err := c.doServerRequest(ctx, http.MethodGet, queryString, nil)
	if err != nil {
		return fmt.Errorf("myjdownloader connect: %w", err)
	}

	// Successful responses from the server are AES-CBC encrypted with loginSecret.
	decrypted, err := decryptAES(string(resp), c.loginSecret)
	if err != nil {
		return fmt.Errorf("myjdownloader connect: decrypt response: %w (raw: %s)", err, string(resp))
	}
	log.Printf("[myjdownloader] connect response (decrypted): %s", string(decrypted))

	var result struct {
		SessionToken string `json:"sessiontoken"`
		RegainToken  string `json:"regaintoken"`
		RID          int64  `json:"rid"`
	}
	if err := json.Unmarshal(decrypted, &result); err != nil {
		return fmt.Errorf("myjdownloader connect: parse response: %w (body: %s)", err, string(decrypted))
	}
	if result.SessionToken == "" {
		return fmt.Errorf("myjdownloader connect: empty session token (response: %s)", string(decrypted))
	}

	c.sessionToken = result.SessionToken

	tokenBytes, err := hex.DecodeString(result.SessionToken)
	if err != nil {
		return fmt.Errorf("myjdownloader connect: decode session token: %w", err)
	}
	c.serverEncryptionKey = updateToken(c.loginSecret, tokenBytes)
	c.deviceEncryptionKey = updateToken(c.deviceSecret, tokenBytes)

	// Resolve device ID.
	deviceID, err := c.listDevicesLocked(ctx)
	if err != nil {
		return fmt.Errorf("myjdownloader connect: list devices: %w", err)
	}
	c.deviceID = deviceID
	return nil
}

// Ping connects and verifies credentials + device are accessible.
func (c *Client) Ping(ctx context.Context) error {
	return c.Connect(ctx)
}

// ListPackages returns all download packages from the configured device.
func (c *Client) ListPackages(ctx context.Context) ([]Package, error) {
	c.mu.Lock()
	if c.sessionToken == "" || c.deviceID == "" {
		c.mu.Unlock()
		if err := c.Connect(ctx); err != nil {
			return nil, err
		}
		c.mu.Lock()
	}
	sessionToken := c.sessionToken
	deviceID := c.deviceID
	devKey := c.deviceEncryptionKey
	c.mu.Unlock()

	reqPayload := map[string]any{
		"bytesLoaded": true,
		"bytesTotal":  true,
		"finished":    true,
		"name":        true,
		"running":     true,
		"status":      true,
		"maxResults":  -1,
		"startAt":     0,
	}

	result, err := c.callDevice(ctx, sessionToken, deviceID, devKey, "/downloadsV2/queryPackages", reqPayload)
	if err != nil {
		// Session may have expired — reconnect once and retry.
		if connectErr := c.Connect(ctx); connectErr != nil {
			return nil, fmt.Errorf("myjdownloader list packages (reconnect): %w", connectErr)
		}
		c.mu.Lock()
		sessionToken = c.sessionToken
		deviceID = c.deviceID
		devKey = c.deviceEncryptionKey
		c.mu.Unlock()
		result, err = c.callDevice(ctx, sessionToken, deviceID, devKey, "/downloadsV2/queryPackages", reqPayload)
		if err != nil {
			return nil, fmt.Errorf("myjdownloader list packages: %w", err)
		}
	}

	var response struct {
		Data []Package `json:"data"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		return nil, fmt.Errorf("myjdownloader list packages: parse response: %w", err)
	}
	return response.Data, nil
}

// listDevicesLocked resolves the configured device name to an ID.
// Caller must hold c.mu.
func (c *Client) listDevicesLocked(ctx context.Context) (string, error) {
	rid := c.rid.Add(1)
	queryString := fmt.Sprintf("/my/listdevices?sessiontoken=%s&rid=%d",
		url.QueryEscape(c.sessionToken), rid)
	sig := signHMAC(queryString, c.serverEncryptionKey)
	queryString += "&signature=" + sig

	resp, err := c.doServerRequest(ctx, http.MethodGet, queryString, nil)
	if err != nil {
		return "", err
	}

	// Response is encrypted with serverEncryptionKey.
	decrypted, err := decryptAES(string(resp), c.serverEncryptionKey)
	if err != nil {
		return "", fmt.Errorf("decrypt listdevices: %w", err)
	}

	var result struct {
		List []Device `json:"list"`
	}
	if err := json.Unmarshal(decrypted, &result); err != nil {
		return "", fmt.Errorf("parse listdevices: %w", err)
	}

	for _, d := range result.List {
		if strings.EqualFold(d.Name, c.DeviceName) {
			return d.ID, nil
		}
	}

	// Build a helpful list of available devices for the error message.
	names := make([]string, 0, len(result.List))
	for _, d := range result.List {
		names = append(names, d.Name)
	}
	return "", fmt.Errorf("device %q not found (available: %s)", c.DeviceName, strings.Join(names, ", "))
}

// callDevice sends an encrypted API call to the JDownloader device via the relay.
func (c *Client) callDevice(ctx context.Context, sessionToken, deviceID string, devKey []byte, action string, payload any) ([]byte, error) {
	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal payload: %w", err)
	}

	encrypted, err := encryptAES(bodyJSON, devKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt payload: %w", err)
	}

	urlPath := fmt.Sprintf("/t_%s_%s%s", sessionToken, deviceID, action)
	rawURL := apiBase + urlPath

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, strings.NewReader(encrypted))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/aesjson-jd; charset=utf-8")

	httpResp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, err
	}
	if httpResp.StatusCode >= 400 {
		return nil, fmt.Errorf("device relay HTTP %d: %s", httpResp.StatusCode, string(respBody))
	}

	decrypted, err := decryptAES(string(respBody), devKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt response: %w", err)
	}
	return decrypted, nil
}

// doServerRequest performs an HTTPS request against the MyJDownloader server.
// Returns the raw response body; callers are responsible for decryption.
// Non-200 responses are parsed as JSON error objects and returned as errors.
func (c *Client) doServerRequest(ctx context.Context, method, queryString string, body io.Reader) ([]byte, error) {
	rawURL := apiBase + queryString

	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		// Non-200 responses are plain-text JSON error objects.
		var errResp struct {
			Src  string `json:"src"`
			Type string `json:"type"`
			Data string `json:"data"`
		}
		if jsonErr := json.Unmarshal(data, &errResp); jsonErr == nil && errResp.Type != "" {
			if errResp.Data != "" {
				return nil, fmt.Errorf("%s: %s", errResp.Type, errResp.Data)
			}
			return nil, fmt.Errorf("%s", errResp.Type)
		}
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(data))
	}

	return data, nil
}

// --- Crypto helpers ---

// deriveSecret computes SHA256(lower(email) + password + suffix).
func deriveSecret(email, password, suffix string) []byte {
	h := sha256.New()
	h.Write([]byte(strings.ToLower(email)))
	h.Write([]byte(password))
	h.Write([]byte(suffix))
	return h.Sum(nil)
}

// updateToken computes SHA256(secret + sessionTokenBytes).
// Uses raw bytes, matching the Python myjdapi reference: bytearray.fromhex(session_token).
func updateToken(secret, sessionTokenBytes []byte) []byte {
	h := sha256.New()
	h.Write(secret)
	h.Write(sessionTokenBytes)
	return h.Sum(nil)
}

// signHMAC returns the hex-encoded HMAC-SHA256 of message under key.
func signHMAC(message string, key []byte) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

// encryptAES AES-128-CBC encrypts data with the given 32-byte key token,
// where bytes 0–15 are the IV and bytes 16–31 are the AES key.
// Returns a base64-encoded ciphertext string.
func encryptAES(data, keyToken []byte) (string, error) {
	iv := keyToken[:16]
	key := keyToken[16:32]

	padded := pkcs7Pad(data, aes.BlockSize)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptAES AES-128-CBC decrypts a base64-encoded ciphertext string.
func decryptAES(b64data string, keyToken []byte) ([]byte, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64data))
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size", len(ciphertext))
	}

	iv := keyToken[:16]
	key := keyToken[16:32]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plaintext := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plaintext, ciphertext)

	return pkcs7Unpad(plaintext)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	pad := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, pad...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid PKCS#7 padding %d", padding)
	}
	return data[:len(data)-padding], nil
}
