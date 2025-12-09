package protocols

// ProtocolListResponse represents the response for listing protocols
type ProtocolListResponse struct {
	Data []*Protocol `json:"data"`
}

// SchemaResponse represents the response for getting a protocol schema
type SchemaResponse struct {
	ProtocolID string      `json:"protocol_id"`
	Schema     interface{} `json:"schema"`
}
