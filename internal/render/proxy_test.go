package render

import "testing"

// Chromium refuses userinfo in --proxy-server, so the credentials must come
// off the endpoint and be answered over CDP instead.
func TestParseProxySplitsCredentialsOffTheEndpoint(t *testing.T) {
	got, err := parseProxy("http://spuser:s3cret@gate.decodo.com:7000")
	if err != nil {
		t.Fatalf("parseProxy: %v", err)
	}
	if got.endpoint != "http://gate.decodo.com:7000" {
		t.Errorf("endpoint = %q, want the host with no credentials", got.endpoint)
	}
	if got.user != "spuser" || got.password != "s3cret" {
		t.Errorf("credentials = %q/%q", got.user, got.password)
	}
}

func TestParseProxyWithoutCredentials(t *testing.T) {
	got, err := parseProxy("http://gate.decodo.com:7000")
	if err != nil {
		t.Fatalf("parseProxy: %v", err)
	}
	if !got.configured() {
		t.Fatal("want a configured proxy")
	}
	if got.user != "" || got.password != "" {
		t.Errorf("want no credentials, got %q/%q", got.user, got.password)
	}
}

func TestParseProxyEmptyIsNotConfigured(t *testing.T) {
	got, err := parseProxy("   ")
	if err != nil {
		t.Fatalf("parseProxy: %v", err)
	}
	if got.configured() {
		t.Fatal("an empty value must mean no proxy")
	}
}

// A typo must be loud at startup rather than a silent direct render that is
// then refused by every publisher.
func TestParseProxyRefusesAnUnparseableValue(t *testing.T) {
	if _, err := parseProxy("://nope"); err == nil {
		t.Fatal("want an error for an unparseable proxy URL")
	}
}

// The error must not quote the value it came from: it holds credentials.
func TestInvalidProxyErrorCarriesNoDetail(t *testing.T) {
	_, err := parseProxy("://spuser:s3cret@nope")
	if err == nil {
		t.Fatal("want an error")
	}
	if msg := err.Error(); msg != "proxy URL is not parseable" {
		t.Errorf("error = %q, want no detail from the value", msg)
	}
}

func TestNewRefusesAnUnparseableProxy(t *testing.T) {
	if _, err := New("", "://nope"); err == nil {
		t.Fatal("want New to refuse an unparseable proxy")
	}
}

func TestNewWithoutProxyHasNone(t *testing.T) {
	b, err := New("", "")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if b.HasProxy() {
		t.Fatal("want no proxy when none is configured")
	}
}
