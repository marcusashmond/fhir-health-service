package models

// https://www.hl7.org/fhir/observation.html
type Observation struct {
	ID                string          `json:"id"`
	Status            string          `json:"status"`
	Code              CodeableConcept `json:"code"`
	Subject           Reference       `json:"subject,omitempty"`
	EffectiveDateTime string          `json:"effectiveDateTime,omitempty"`
	ValueQuantity     *Quantity       `json:"valueQuantity,omitempty"`
}

type Quantity struct {
	Value float64 `json:"value"`
	Unit  string  `json:"unit,omitempty"`
}

type CodeableConcept struct {
	Coding []Coding `json:"coding,omitempty"`
	Text   string   `json:"text,omitempty"`
}

type Coding struct {
	System  string `json:"system,omitempty"`
	Code    string `json:"code,omitempty"`
	Display string `json:"display,omitempty"`
}

type Reference struct {
	Reference string `json:"reference,omitempty"`
}
