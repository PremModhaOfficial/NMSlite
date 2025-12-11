package protocols

// ProtocolListResponse represents the response for listing protocols
type ProtocolListResponse struct {
	Data []*Protocol `json:"data"`
}
