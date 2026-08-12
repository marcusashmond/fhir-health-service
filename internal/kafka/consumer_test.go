package kafka

import (
	"bytes"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/marcusashmond/fhir-health-service/internal/models"
)

func observationWithValue(code string, value float64) *models.Observation {
	return &models.Observation{
		ID:      "obs1",
		Status:  "final",
		Subject: models.Reference{Reference: "Patient/p1"},
		Code:    models.CodeableConcept{Coding: []models.Coding{{System: "http://loinc.org", Code: code}}},
		ValueQuantity: &models.Quantity{
			Value: value,
			Unit:  "mmHg",
		},
	}
}

func TestCheckAnomaly(t *testing.T) {
	tests := []struct {
		name        string
		observation *models.Observation
		wantAlert   bool
	}{
		{
			name:        "within normal range",
			observation: observationWithValue("8480-6", 110),
			wantAlert:   false,
		},
		{
			name:        "at lower bound",
			observation: observationWithValue("8480-6", 90),
			wantAlert:   false,
		},
		{
			name:        "at upper bound",
			observation: observationWithValue("8480-6", 140),
			wantAlert:   false,
		},
		{
			name:        "below normal range",
			observation: observationWithValue("8480-6", 60),
			wantAlert:   true,
		},
		{
			name:        "above normal range",
			observation: observationWithValue("8480-6", 200),
			wantAlert:   true,
		},
		{
			name:        "unknown code is ignored",
			observation: observationWithValue("9999-9", 500),
			wantAlert:   false,
		},
		{
			name: "no value quantity is ignored",
			observation: &models.Observation{
				ID:      "obs2",
				Status:  "final",
				Subject: models.Reference{Reference: "Patient/p1"},
				Code:    models.CodeableConcept{Coding: []models.Coding{{System: "http://loinc.org", Code: "8480-6"}}},
			},
			wantAlert: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			log.SetOutput(&buf)
			t.Cleanup(func() { log.SetOutput(os.Stderr) })

			checkAnomaly(tt.observation)

			gotAlert := strings.Contains(buf.String(), "ALERT")
			if gotAlert != tt.wantAlert {
				t.Errorf("checkAnomaly() alert logged = %v, want %v (log: %q)", gotAlert, tt.wantAlert, buf.String())
			}
		})
	}
}

func TestObservationCode(t *testing.T) {
	tests := []struct {
		name        string
		observation *models.Observation
		want        string
	}{
		{
			name: "prefers loinc coding",
			observation: &models.Observation{
				Code: models.CodeableConcept{Coding: []models.Coding{
					{System: "http://snomed.info/sct", Code: "1234"},
					{System: "http://loinc.org", Code: "8480-6"},
				}},
			},
			want: "8480-6",
		},
		{
			name: "falls back to first coding when no loinc",
			observation: &models.Observation{
				Code: models.CodeableConcept{Coding: []models.Coding{
					{System: "http://snomed.info/sct", Code: "1234"},
				}},
			},
			want: "1234",
		},
		{
			name:        "no codings returns empty string",
			observation: &models.Observation{},
			want:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := observationCode(tt.observation)
			if got != tt.want {
				t.Errorf("observationCode() = %q, want %q", got, tt.want)
			}
		})
	}
}
