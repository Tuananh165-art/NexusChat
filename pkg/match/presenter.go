package match

import (
	"encoding/json"
)

type MatchResultPresenter struct {
	ChannelID   string `json:"channel_id"`
	AccessToken string `json:"access_token"`
	Kind        string `json:"kind"`
	PeerUserID  string `json:"peer_user_id,omitempty"`
}

func (m *MatchResultPresenter) Encode() []byte {
	result, _ := json.Marshal(m)
	return result
}
