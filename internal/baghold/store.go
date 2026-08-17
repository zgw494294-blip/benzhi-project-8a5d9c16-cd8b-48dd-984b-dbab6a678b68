package baghold

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
)

type ErrorKind string

const (
	ErrorInvalidInput ErrorKind = "invalid_input"
	ErrorNotFound     ErrorKind = "not_found"
	ErrorConflict     ErrorKind = "conflict"
)

var (
	ErrInvalidInput = errors.New("invalid input")
	ErrNotFound     = errors.New("test not found")
	ErrConflict     = errors.New("test state conflict")
)

// DomainError preserves the category of a rejected operation for the HTTP layer.
type DomainError struct {
	Kind    ErrorKind
	Field   string
	Message string
	cause   error
}

func (e *DomainError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return fmt.Sprintf("%s: %s", e.Field, e.Message)
}

func (e *DomainError) Unwrap() error { return e.cause }

type Status string

const (
	StatusActive Status = "active"
	StatusPassed Status = "passed"
	StatusFailed Status = "failed"
)

type Sample struct {
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	VacuumKPa      float64 `json:"vacuum_kpa"`
}

type Assessment struct {
	Result        Status  `json:"result"`
	VacuumLossKPa float64 `json:"vacuum_loss_kpa"`
}

type HoldTest struct {
	ID                   string      `json:"id"`
	BagID                string      `json:"bag_id"`
	MinimumHoldSeconds   float64     `json:"min_hold_seconds"`
	MaximumVacuumLossKPa float64     `json:"max_vacuum_loss_kpa"`
	OperatorNote         *string     `json:"operator_note,omitempty"`
	Status               Status      `json:"status"`
	Samples              []Sample    `json:"samples"`
	Assessment           *Assessment `json:"assessment,omitempty"`
}

type CreateInput struct {
	BagID                string  `json:"bag_id"`
	MinimumHoldSeconds   float64 `json:"min_hold_seconds"`
	MaximumVacuumLossKPa float64 `json:"max_vacuum_loss_kpa"`
	OperatorNote         *string `json:"operator_note,omitempty"`
}

type SampleInput struct {
	ElapsedSeconds float64 `json:"elapsed_seconds"`
	VacuumKPa      float64 `json:"vacuum_kpa"`
}

type Store struct {
	mu     sync.RWMutex
	tests  map[string]HoldTest
	nextID uint64
}

func NewStore() *Store {
	return &Store{tests: make(map[string]HoldTest)}
}

func (s *Store) Create(ctx context.Context, input CreateInput) (HoldTest, error) {
	if err := validateCreate(input); err != nil {
		return HoldTest{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, existing := range s.tests {
		if existing.BagID == input.BagID {
			return HoldTest{}, domainError(ErrorConflict, ErrConflict, "bag_id", "already exists")
		}
	}
	if err := ctx.Err(); err != nil {
		return HoldTest{}, err
	}

	s.nextID++
	id := fmt.Sprintf("test-%06d", s.nextID)
	record := HoldTest{
		ID:                   id,
		BagID:                input.BagID,
		MinimumHoldSeconds:   input.MinimumHoldSeconds,
		MaximumVacuumLossKPa: input.MaximumVacuumLossKPa,
		OperatorNote:         cloneStringPointer(input.OperatorNote),
		Status:               StatusActive,
		Samples:              []Sample{},
	}
	s.tests[id] = record
	return cloneHoldTest(record), nil
}

func (s *Store) AddSample(ctx context.Context, id string, input SampleInput) (HoldTest, error) {
	if err := validateSample(input); err != nil {
		return HoldTest{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.tests[id]
	if !ok {
		return HoldTest{}, domainError(ErrorNotFound, ErrNotFound, "", "test not found")
	}
	if record.Status != StatusActive {
		return HoldTest{}, domainError(ErrorConflict, ErrConflict, "status", "test has already been assessed")
	}
	if len(record.Samples) > 0 && input.ElapsedSeconds <= record.Samples[len(record.Samples)-1].ElapsedSeconds {
		return HoldTest{}, domainError(ErrorInvalidInput, ErrInvalidInput, "elapsed_seconds", "must be strictly increasing")
	}
	if err := ctx.Err(); err != nil {
		return HoldTest{}, err
	}

	record.Samples = append(record.Samples, Sample{
		ElapsedSeconds: input.ElapsedSeconds,
		VacuumKPa:      input.VacuumKPa,
	})
	s.tests[id] = record
	return cloneHoldTest(record), nil
}

func (s *Store) Assess(ctx context.Context, id string) (HoldTest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.tests[id]
	if !ok {
		return HoldTest{}, domainError(ErrorNotFound, ErrNotFound, "", "test not found")
	}
	if record.Status != StatusActive {
		return HoldTest{}, domainError(ErrorConflict, ErrConflict, "status", "test has already been assessed")
	}
	if len(record.Samples) < 2 {
		return HoldTest{}, domainError(ErrorConflict, ErrConflict, "samples", "at least two samples are required")
	}
	last := record.Samples[len(record.Samples)-1]
	if last.ElapsedSeconds < record.MinimumHoldSeconds {
		return HoldTest{}, domainError(ErrorConflict, ErrConflict, "elapsed_seconds", "minimum hold duration has not been reached")
	}
	if err := ctx.Err(); err != nil {
		return HoldTest{}, err
	}

	loss := record.Samples[0].VacuumKPa - last.VacuumKPa
	result := StatusPassed
	if loss > record.MaximumVacuumLossKPa {
		result = StatusFailed
	}
	record.Status = result
	record.Assessment = &Assessment{Result: result, VacuumLossKPa: loss}
	s.tests[id] = record
	return cloneHoldTest(record), nil
}

func (s *Store) Get(id string) (HoldTest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.tests[id]
	if !ok {
		return HoldTest{}, domainError(ErrorNotFound, ErrNotFound, "", "test not found")
	}
	return cloneHoldTest(record), nil
}

func validateCreate(input CreateInput) error {
	if strings.TrimSpace(input.BagID) == "" {
		return domainError(ErrorInvalidInput, ErrInvalidInput, "bag_id", "is required")
	}
	if !validNonNegative(input.MinimumHoldSeconds) {
		return domainError(ErrorInvalidInput, ErrInvalidInput, "min_hold_seconds", "must be a finite nonnegative number")
	}
	if !validNonNegative(input.MaximumVacuumLossKPa) {
		return domainError(ErrorInvalidInput, ErrInvalidInput, "max_vacuum_loss_kpa", "must be a finite nonnegative number")
	}
	return nil
}

func validateSample(input SampleInput) error {
	if !validNonNegative(input.ElapsedSeconds) {
		return domainError(ErrorInvalidInput, ErrInvalidInput, "elapsed_seconds", "must be a finite nonnegative number")
	}
	if !validNonNegative(input.VacuumKPa) {
		return domainError(ErrorInvalidInput, ErrInvalidInput, "vacuum_kpa", "must be a finite nonnegative number")
	}
	return nil
}

func validNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func domainError(kind ErrorKind, cause error, field, message string) error {
	return &DomainError{Kind: kind, Field: field, Message: message, cause: cause}
}

func cloneHoldTest(record HoldTest) HoldTest {
	snapshot := record
	snapshot.OperatorNote = cloneStringPointer(record.OperatorNote)
	if record.Samples != nil {
		snapshot.Samples = make([]Sample, len(record.Samples))
		copy(snapshot.Samples, record.Samples)
	}
	if record.Assessment != nil {
		assessment := *record.Assessment
		snapshot.Assessment = &assessment
	}
	return snapshot
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
