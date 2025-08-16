package api

type PaymentRequest struct {
	CorrelationID string  `json:"correlationId,omitempty"`
	RequestedAt   string  `json:"requestedAt"`
	Amount        float64 `json:"amount"`
	ServerType    string  `json:"serverType"` // "default" or "fallback"
}

type Summary struct {
	TotalRequests int     `json:"totalRequests"`
	TotalAmount   float64 `json:"totalAmount"`
}
