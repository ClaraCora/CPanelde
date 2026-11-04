package socks

import (
	"github.com/sagernet/sing/common/auth"
)

func (h *Inbound) UpdateUsers(users []auth.User) error {
	// Each handshake retains one immutable snapshot while updates publish a new one.
	h.authenticator.Store(auth.NewAuthenticator(users))
	return nil
}
