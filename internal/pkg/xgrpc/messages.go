package xgrpc

// AckResponse is a generic RPC success response.
type AckResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

