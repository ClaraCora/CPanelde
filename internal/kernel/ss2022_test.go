package kernel

import (
	"encoding/base64"
	"testing"
)

func TestSS2022UserKey(t *testing.T) {
	tests := []struct {
		name   string
		method string
		size   int
	}{
		{name: "aes-128", method: "2022-blake3-aes-128-gcm", size: 16},
		{name: "aes-256", method: "2022-blake3-aes-256-gcm", size: 32},
	}
	const udid = "279d4f89-3a2c-488d-a67c-2d39a72acdde"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := SS2022UserKey(test.method, udid)
			if !ok {
				t.Fatal("SS2022UserKey() did not recognize method")
			}
			want := base64.StdEncoding.EncodeToString([]byte(udid[:test.size]))
			if got != want {
				t.Fatalf("SS2022UserKey() = %q, want %q", got, want)
			}
		})
	}
}

func TestSS2022UserKeyRejectsTraditionalMethod(t *testing.T) {
	if _, ok := SS2022UserKey("aes-128-gcm", "user-udid"); ok {
		t.Fatal("SS2022UserKey() accepted a traditional Shadowsocks method")
	}
}
