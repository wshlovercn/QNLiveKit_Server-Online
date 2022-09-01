package model

type StreamHistoryItem struct {
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	ClientIP string `json:"client_ip"`
	ServerIP string `json:"server_ip"`
	MediaUrl string `json:"media_url"`
}
