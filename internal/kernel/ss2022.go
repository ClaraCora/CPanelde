package kernel

import "encoding/base64"

var ss2022UserKeySizes = map[string]int{
	"2022-blake3-aes-128-gcm":       16,
	"2022-blake3-aes-256-gcm":       32,
	"2022-blake3-chacha20-poly1305": 32,
}

func IsSS2022Method(method string) bool {
	_, ok := ss2022UserKeySizes[method]
	return ok
}

// SS2022UserKey converts a user UDID into the fixed-size Base64 key expected
// by Shadowsocks 2022 multi-user servers and clients.
func SS2022UserKey(method, udid string) (string, bool) {
	size, ok := ss2022UserKeySizes[method]
	if !ok {
		return "", false
	}
	raw := make([]byte, size)
	copy(raw, udid)
	return base64.StdEncoding.EncodeToString(raw), true
}
