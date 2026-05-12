package models

type PoliticianTrade struct {
	Name      string `json:"name"`
	Role      string `json:"role"`
	Party     string `json:"party"`
	Action    string `json:"action"`
	Amount    string `json:"amount"`
	FiledDate string `json:"filed_date"`
}
