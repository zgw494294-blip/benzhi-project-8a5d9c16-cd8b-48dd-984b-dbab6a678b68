package baghold

import (
	"context"
	"errors"
	"math"
	"testing"
)

func TestStorePreservesOptionalNoteAndDefensiveSamples(t *testing.T) {
	store := NewStore()
	emptyNote := ""
	withNote, err := store.Create(context.Background(), CreateInput{
		BagID:                "BAG-EMPTY-NOTE",
		MinimumHoldSeconds:   10,
		MaximumVacuumLossKPa: 1,
		OperatorNote:         &emptyNote,
	})
	if err != nil {
		t.Fatalf("create with empty note: %v", err)
	}
	withoutNote, err := store.Create(context.Background(), CreateInput{
		BagID:                "BAG-NO-NOTE",
		MinimumHoldSeconds:   10,
		MaximumVacuumLossKPa: 1,
	})
	if err != nil {
		t.Fatalf("create without note: %v", err)
	}
	if withNote.OperatorNote == nil || *withNote.OperatorNote != "" {
		t.Fatalf("empty note was not preserved: %#v", withNote.OperatorNote)
	}
	if withoutNote.OperatorNote != nil {
		t.Fatalf("omitted note was not preserved: %#v", withoutNote.OperatorNote)
	}
	if withNote.Samples == nil || len(withNote.Samples) != 0 {
		t.Fatalf("new test should expose an empty sample slice: %#v", withNote.Samples)
	}

	added, err := store.AddSample(context.Background(), withNote.ID, SampleInput{ElapsedSeconds: 0, VacuumKPa: 30})
	if err != nil {
		t.Fatalf("add sample: %v", err)
	}
	added.Samples[0].VacuumKPa = 1
	fetched, err := store.Get(withNote.ID)
	if err != nil {
		t.Fatalf("get test: %v", err)
	}
	if fetched.Samples[0].VacuumKPa != 30 {
		t.Fatalf("returned sample aliased store: %#v", fetched.Samples)
	}
	fetched.Samples[0].VacuumKPa = 2
	fetchedAgain, err := store.Get(withNote.ID)
	if err != nil {
		t.Fatalf("get test again: %v", err)
	}
	if fetchedAgain.Samples[0].VacuumKPa != 30 {
		t.Fatalf("get snapshot aliased store: %#v", fetchedAgain.Samples)
	}
	if withoutNote.ID == withNote.ID {
		t.Fatalf("test IDs must be unique")
	}
}

func TestStoreValidatesInputsAndTypedErrors(t *testing.T) {
	store := NewStore()
	for _, input := range []CreateInput{
		{BagID: "", MinimumHoldSeconds: 1, MaximumVacuumLossKPa: 1},
		{BagID: "negative-duration", MinimumHoldSeconds: -1, MaximumVacuumLossKPa: 1},
		{BagID: "infinite-loss", MinimumHoldSeconds: 1, MaximumVacuumLossKPa: math.Inf(1)},
	} {
		if _, err := store.Create(context.Background(), input); err == nil {
			t.Fatalf("expected create validation error for %#v", input)
		} else {
			var domainErr *DomainError
			if !errors.As(err, &domainErr) || domainErr.Kind != ErrorInvalidInput || !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected typed invalid-input error, got %T %v", err, err)
			}
		}
	}

	record, err := store.Create(context.Background(), CreateInput{BagID: "BAG-1", MinimumHoldSeconds: 1, MaximumVacuumLossKPa: 1})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.Create(context.Background(), CreateInput{BagID: "BAG-1", MinimumHoldSeconds: 1, MaximumVacuumLossKPa: 1}); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected duplicate bag conflict, got %v", err)
	}
	if _, err := store.Get("missing"); err == nil || !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected missing-test error, got %v", err)
	}
	if _, err := store.AddSample(context.Background(), record.ID, SampleInput{ElapsedSeconds: 0, VacuumKPa: math.NaN()}); err == nil || !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected sample validation error, got %v", err)
	}
}

func TestStoreAssessesOnceAndLocksState(t *testing.T) {
	store := NewStore()
	record, err := store.Create(context.Background(), CreateInput{
		BagID:                "BAG-ASSESS",
		MinimumHoldSeconds:   60,
		MaximumVacuumLossKPa: 3,
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := store.Assess(context.Background(), record.ID); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected too-few-samples conflict, got %v", err)
	}
	for _, sample := range []SampleInput{{ElapsedSeconds: 0, VacuumKPa: 30}, {ElapsedSeconds: 30, VacuumKPa: 29}, {ElapsedSeconds: 60, VacuumKPa: 27.5}} {
		if _, err := store.AddSample(context.Background(), record.ID, sample); err != nil {
			t.Fatalf("add sample %#v: %v", sample, err)
		}
	}
	assessed, err := store.Assess(context.Background(), record.ID)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if assessed.Status != StatusPassed || assessed.Assessment == nil || assessed.Assessment.VacuumLossKPa != 2.5 {
		t.Fatalf("unexpected assessment: %#v", assessed)
	}
	if _, err := store.AddSample(context.Background(), record.ID, SampleInput{ElapsedSeconds: 61, VacuumKPa: 27}); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected post-assessment sample conflict, got %v", err)
	}
	if _, err := store.Assess(context.Background(), record.ID); err == nil || !errors.Is(err, ErrConflict) {
		t.Fatalf("expected post-assessment conflict, got %v", err)
	}
	fetched, err := store.Get(record.ID)
	if err != nil {
		t.Fatalf("get assessed test: %v", err)
	}
	if len(fetched.Samples) != 3 || fetched.Status != StatusPassed || fetched.Assessment.VacuumLossKPa != 2.5 {
		t.Fatalf("assessment state mutated: %#v", fetched)
	}
}

func TestStoreCancellationDoesNotCommit(t *testing.T) {
	store := NewStore()
	createContext, cancelCreate := context.WithCancel(context.Background())
	cancelCreate()
	if _, err := store.Create(createContext, CreateInput{BagID: "CANCELED", MinimumHoldSeconds: 1, MaximumVacuumLossKPa: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled create, got %v", err)
	}

	record, err := store.Create(context.Background(), CreateInput{BagID: "ACTIVE", MinimumHoldSeconds: 1, MaximumVacuumLossKPa: 1})
	if err != nil {
		t.Fatalf("create active test: %v", err)
	}
	addContext, cancelAdd := context.WithCancel(context.Background())
	cancelAdd()
	if _, err := store.AddSample(addContext, record.ID, SampleInput{ElapsedSeconds: 0, VacuumKPa: 30}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled sample, got %v", err)
	}
	fetched, err := store.Get(record.ID)
	if err != nil {
		t.Fatalf("get active test: %v", err)
	}
	if len(fetched.Samples) != 0 {
		t.Fatalf("canceled sample changed state: %#v", fetched.Samples)
	}
}
