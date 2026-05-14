package blob

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type S3Config struct {
	Endpoint        string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	Region          string
	UseSSL          bool
}

type S3Store struct {
	httpClient      *http.Client
	endpoint        string
	accessKeyID     string
	secretAccessKey string
	bucket          string
	region          string
	scheme          string
}

func NewS3Store(ctx context.Context, cfg S3Config) (*S3Store, error) {
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	scheme := "http"
	if cfg.UseSSL {
		scheme = "https"
	}
	s := &S3Store{httpClient: &http.Client{Timeout: 30 * time.Second}, endpoint: strings.TrimPrefix(strings.TrimPrefix(cfg.Endpoint, "https://"), "http://"), accessKeyID: cfg.AccessKeyID, secretAccessKey: cfg.SecretAccessKey, bucket: cfg.Bucket, region: cfg.Region, scheme: scheme}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *S3Store) ensureBucket(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, fmt.Sprintf("%s://%s/%s", s.scheme, s.endpoint, s.bucket), nil)
	if err != nil {
		return err
	}
	s.sign(req, nil)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("check bucket: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}
	if resp.StatusCode != http.StatusNotFound {
		return fmt.Errorf("check bucket: status %d", resp.StatusCode)
	}
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, fmt.Sprintf("%s://%s/%s", s.scheme, s.endpoint, s.bucket), nil)
	if err != nil {
		return err
	}
	s.sign(req, nil)
	resp, err = s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("create bucket: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("create bucket: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *S3Store) Put(ctx context.Context, key string, data []byte) error {
	req, err := s.newRequest(ctx, http.MethodPut, key, data)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/pdf")
	return s.doNoBody(req, http.StatusOK)
}

func (s *S3Store) Get(ctx context.Context, key string) ([]byte, error) {
	req, err := s.newRequest(ctx, http.MethodGet, key, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get object: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("get object: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read object: %w", err)
	}
	return data, nil
}

func (s *S3Store) Delete(ctx context.Context, key string) error {
	req, err := s.newRequest(ctx, http.MethodDelete, key, nil)
	if err != nil {
		return err
	}
	return s.doNoBody(req, http.StatusNoContent)
}

func (s *S3Store) doNoBody(req *http.Request, ok int) error {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("object request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != ok && resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("object request: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (s *S3Store) newRequest(ctx context.Context, method, key string, data []byte) (*http.Request, error) {
	escapedKey := strings.TrimLeft(key, "/")
	u := fmt.Sprintf("%s://%s/%s/%s", s.scheme, s.endpoint, s.bucket, pathEscapeObject(escapedKey))
	var body io.Reader
	if data != nil {
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	s.sign(req, data)
	return req, nil
}

func (s *S3Store) sign(req *http.Request, payload []byte) {
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	dateStamp := now.Format("20060102")
	payloadHashBytes := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(payloadHashBytes[:])
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n", req.URL.Host, payloadHash, amzDate)
	canonicalURI := req.URL.EscapedPath()
	canonicalRequest := strings.Join([]string{req.Method, canonicalURI, "", canonicalHeaders, signedHeaders, payloadHash}, "\n")
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, s.region)
	canonicalHash := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := strings.Join([]string{"AWS4-HMAC-SHA256", amzDate, credentialScope, hex.EncodeToString(canonicalHash[:])}, "\n")
	signingKey := awsSigningKey(s.secretAccessKey, dateStamp, s.region, "s3")
	sigMAC := hmac.New(sha256.New, signingKey)
	sigMAC.Write([]byte(stringToSign))
	signature := hex.EncodeToString(sigMAC.Sum(nil))
	req.Header.Set("Authorization", fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s", s.accessKeyID, credentialScope, signedHeaders, signature))
}

func awsSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, "aws4_request")
}

func hmacSHA256(key []byte, data string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(data))
	return mac.Sum(nil)
}

func pathEscapeObject(key string) string {
	parts := strings.Split(key, "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}
