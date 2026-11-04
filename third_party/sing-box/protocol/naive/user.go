package naive

import (
	"github.com/sagernet/sing/common/auth"
)

func (n *Inbound) UpdateUsers(users []auth.User) error {
	n.authenticator.Store(auth.NewAuthenticator(users))
	return nil
}
