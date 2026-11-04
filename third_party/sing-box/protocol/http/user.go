package http

import (
	"github.com/sagernet/sing/common/auth"
)

func (h *Inbound) UpdateUsers(users []auth.User) error {
	h.authenticator.Store(auth.NewAuthenticator(users))
	return nil
}
