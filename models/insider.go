package models

type InsiderTrade struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Action    string `json:"action"`
	Note      string `json:"note,omitempty"`
	Amount    string `json:"amount"`
	FiledDate string `json:"filed_date"`
}
