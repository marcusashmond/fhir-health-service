package models

// https://www.hl7.org/fhir/patient.html
type Patient struct {
	ID         string       `json:"id"`
	Identifier []Identifier `json:"identifier,omitempty"`
	Name       []HumanName  `json:"name,omitempty"`
	Gender     string       `json:"gender,omitempty"`
	BirthDate  string       `json:"birthDate,omitempty"`
	Address    []Address    `json:"address,omitempty"`
}

type Identifier struct {
	System string `json:"system,omitempty"`
	Value  string `json:"value,omitempty"`
}

type HumanName struct {
	Family string   `json:"family,omitempty"`
	Given  []string `json:"given,omitempty"`
}

type Address struct {
	Line       []string `json:"line,omitempty"`
	City       string   `json:"city,omitempty"`
	State      string   `json:"state,omitempty"`
	PostalCode string   `json:"postalCode,omitempty"`
	Country    string   `json:"country,omitempty"`
}
