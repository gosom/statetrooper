/*
MIT License

Copyright (c) 2023 Hisham Khalifa

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
*/

package statetrooper

import (
	"encoding/json"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"
)

// CustomStateEnum represents a custom state enum for testing
type CustomStateEnum string

// Enum values for custom state
const (
	CustomStateEnumA CustomStateEnum = "A"
	CustomStateEnumB CustomStateEnum = "B"
	CustomStateEnumC CustomStateEnum = "C"
	CustomStateEnumD CustomStateEnum = "D"
)

func (c CustomStateEnum) String() string {
	return string(c)
}

func Test_canTransition(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)
	fsm.AddRule(CustomStateEnumC, CustomStateEnumD)

	tests := []struct {
		currentState CustomStateEnum
		targetState  CustomStateEnum
		expected     bool
	}{
		{CustomStateEnumA, CustomStateEnumB, true},
		{CustomStateEnumA, CustomStateEnumC, false},
		{CustomStateEnumB, CustomStateEnumA, false},
		{CustomStateEnumB, CustomStateEnumC, true},
		{CustomStateEnumC, CustomStateEnumA, false},
		{CustomStateEnumC, CustomStateEnumB, false},
		{CustomStateEnumC, CustomStateEnumC, false},
		{CustomStateEnumC, CustomStateEnumD, true},
	}

	for _, test := range tests {
		result := fsm.canTransition(&test.currentState, &test.targetState)
		if result != test.expected {
			t.Errorf("canTransition(%v, %v) = %v, expected %v", test.currentState, test.targetState, result, test.expected)
		}
	}
}

func Test_transition(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	tests := []struct {
		targetState CustomStateEnum
		expected    CustomStateEnum
		wantErr     bool
	}{
		{CustomStateEnumB, CustomStateEnumB, false}, // Valid state transition
		{CustomStateEnumB, CustomStateEnumB, true},  // Invalid state transition (already in target state)
		{CustomStateEnumA, CustomStateEnumB, true},  // Invalid state transition (no transition from current state to target state)
		{CustomStateEnumC, CustomStateEnumC, false}, // Valid state transition
		{CustomStateEnumD, CustomStateEnumC, true},  // Invalid state transition (no transition from current state to target state)
	}

	for _, test := range tests {
		newState, err := fsm.Transition(test.targetState, nil)
		if (err != nil) != test.wantErr {
			t.Errorf("Transition(%v, %v) returned error: %v, wantErr: %v", fsm.currentState, test.targetState, err, test.wantErr)
		}

		if fsm.currentState != test.expected {
			t.Errorf("Transition(%v, %v) did not update the current state to %v", fsm.currentState, test.targetState, test.expected)
		}

		if newState == fsm.currentState && newState != test.expected {
			t.Errorf("Transition(%v, %v) did not return the expected new state of %v", fsm.currentState, test.targetState, test.expected)
		}
	}
}

func Test_transitionTracking(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	metadata1 := map[string]string{
		"requested_by":  "Mahmoud",
		"logic_version": "1.0",
	}

	// Perform the first transition
	_, err := fsm.Transition(CustomStateEnumB, metadata1)
	if err != nil {
		t.Errorf("Transition(%v, %v) returned an error: %v", fsm.currentState, CustomStateEnumB, err)
	}

	time.Sleep(1 * time.Millisecond) // Add slight delay between transitions

	metadata2 := map[string]string{
		"requested_by":  "John",
		"logic_version": "1.1",
	}

	// Perform the second transition
	_, err = fsm.Transition(CustomStateEnumC, metadata2)
	if err != nil {
		t.Errorf("Transition(%v, %v) returned an error: %v", fsm.currentState, CustomStateEnumC, err)
	}

	// Verify the number of entries in the transition tracker
	if len(fsm.transitions) != 2 {
		t.Errorf("Transition tracker does not contain the expected number of entries. Got %d, expected 2", len(fsm.transitions))
	}

	// Get the transition timestamps in order
	timestamps := make([]time.Time, 0, len(fsm.transitions))
	for _, t := range fsm.transitions {
		timestamps = append(timestamps, t.Timestamp)
	}
	sort.Slice(timestamps, func(i, j int) bool {
		return timestamps[i].Before(timestamps[j])
	})

	// Check each transition in the tracker
	expectedTransitions := []struct {
		FromState CustomStateEnum
		ToState   CustomStateEnum
		Timestamp time.Time
		Metadata  map[string]string
	}{
		{
			FromState: CustomStateEnumA,
			ToState:   CustomStateEnumB,
			Timestamp: timestamps[0],
			Metadata:  metadata1,
		},
		{
			FromState: CustomStateEnumB,
			ToState:   CustomStateEnumC,
			Timestamp: timestamps[1],
			Metadata:  metadata2,
		},
	}

	for i, tr := range fsm.transitions {
		expected := expectedTransitions[i]

		if tr.FromState != expected.FromState {
			t.Errorf("Transition tracker has incorrect FromState. Got %v, expected %v", tr.FromState, expected.FromState)
		}

		if tr.ToState != expected.ToState {
			t.Errorf("Transition tracker has incorrect ToState. Got %v, expected %v", tr.ToState, expected.ToState)
		}

		// Allow a small delta in the timestamp comparison due to slight time difference
		if tr.Timestamp.IsZero() {
			t.Errorf("Transition tracker has zero Timestamp. Got %v.", tr.Timestamp)
		}

		// Deep compare metadata
		if !reflect.DeepEqual(tr.Metadata, expected.Metadata) {
			t.Errorf("Transition tracker has incorrect Metadata. Got %v, expected %v", tr.Metadata, expected.Metadata)
		}
	}
}

func Test_concurrencyRaceCondition(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	var wg sync.WaitGroup

	numGoroutines := 100

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			for j := 0; j < 1000; j++ {
				fsm.Transition(CustomStateEnumB, nil)
				fsm.Transition(CustomStateEnumC, nil)
			}
		}()
	}

	// Wait for all goroutines to finish
	wg.Wait()
}

func Test_generateMermaidRulesDiagram(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	fsm.Transition(
		CustomStateEnumB,
		map[string]string{
			"requested_by":  "Mahmoud",
			"logic_version": "1.0",
		})

	fsm.Transition(
		CustomStateEnumC,
		map[string]string{
			"requested_by":  "John",
			"logic_version": "1.1",
		})

	d, err := fsm.GenerateMermaidRulesDiagram()

	// check that the diagram is as expected

	if err != nil {
		t.Errorf("GenerateMermaidRulesDiagram() returned an error: %v", err)
	}

	expectedDiagram := "graph LR;\nA\nB\nA --> B;\nB --> C;\n"

	if d != expectedDiagram {
		t.Errorf("GenerateMermaidRulesDiagram() returned an unexpected diagram:\n%s\nexpected:\n%s", d, expectedDiagram)

	}
}

func Test_gnerateMermaidTransitionHistoryDiagram(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	fsm.Transition(
		CustomStateEnumB,
		map[string]string{
			"requested_by":  "Mahmoud",
			"logic_version": "1.0",
		})

	fsm.Transition(
		CustomStateEnumC,
		map[string]string{
			"requested_by":  "John",
			"logic_version": "1.1",
		})

	d, err := fsm.GenerateMermaidTransitionHistoryDiagram()

	// check that the diagram is as expected

	if err != nil {
		t.Errorf("GenerateMermaidTransitionHistoryDiagram() returned an error: %v", err)
	}

	expectedDiagram := "graph TD;\nA;\nB;\nC;\n\nA -->|1| B;\nB -->|2| C;\n"

	if d != expectedDiagram {
		t.Errorf("GenerateMermaidTransitionHistoryDiagram() returned an unexpected diagram:\n%s\nexpected:\n%s", d, expectedDiagram)

	}
}

func Test_marshalJSON(t *testing.T) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	fsm.Transition(
		CustomStateEnumB,
		map[string]string{
			"requested_by":  "Mahmoud",
			"logic_version": "1.0",
		})

	fsm.Transition(
		CustomStateEnumC,
		map[string]string{
			"requested_by":  "John",
			"logic_version": "1.1",
		})

	_, err := json.Marshal(fsm)

	if err != nil {
		t.Errorf("JSON() returned an error: %v", err)
	}
}

func Test_unmarshalJSON(t *testing.T) {
	// Create a sample FSM JSON data
	jsonData := []byte(`{
		"current_state": "stateB",
		"transitions": [
			{
				"from_state": "stateA",
				"to_state": "stateB",
				"timestamp": "2022-01-01T12:00:00Z",
				"metadata": {
					"reason": "Transition from stateA to stateB"
				}
			}
		]
	}`)

	// Create an FSM instance to test
	fsm := &FSM[string]{
		currentState: "initial",
		maxHistory:   10,
	}

	// Unmarshal the JSON data into the FSM
	err := json.Unmarshal(jsonData, &fsm)
	if err != nil {
		t.Errorf("UnmarshalJSON failed: %v", err)
	}

	// Verify the updated FSM state and transitions
	expectedState := "stateB"
	if fsm.currentState != expectedState {
		t.Errorf("Unexpected currentState. Expected: %s, Got: %s", expectedState, fsm.currentState)
	}

	tp, err := time.Parse(time.RFC3339, "2022-01-01T12:00:00Z")

	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expectedTransition := Transition[string]{
		FromState: "stateA",
		ToState:   "stateB",
		Timestamp: tp,
		Metadata:  map[string]string{"reason": "Transition from stateA to stateB"},
	}
	if !reflect.DeepEqual(fsm.transitions, []Transition[string]{expectedTransition}) {
		t.Errorf("Unexpected transitions. Expected: %v, Got: %v", []Transition[string]{expectedTransition}, fsm.transitions)
	}
}

func Test_withCustomTimeProvider(t *testing.T) {
	var (
		staticTime = time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC)
	)
	fsm := NewFSM[CustomStateEnum](
		CustomStateEnumA,
		10,
		WithTimeProvider[CustomStateEnum](func() time.Time {
			return staticTime
		}),
	)

	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumC)

	metadata1 := map[string]string{
		"requested_by":  "Mahmoud",
		"logic_version": "1.0",
	}

	// Perform the first transition
	_, err := fsm.Transition(CustomStateEnumB, metadata1)
	if err != nil {
		t.Errorf("Transition(%v, %v) returned an error: %v", fsm.currentState, CustomStateEnumB, err)
	}

	_, err = fsm.Transition(CustomStateEnumC, metadata1)
	if err != nil {
		t.Errorf("Transition(%v, %v) returned an error: %v", fsm.currentState, CustomStateEnumC, err)
	}

	transitions := fsm.Transitions()

	if len(transitions) != 2 {
		t.Errorf("Transitions() returned an unexpected number of transitions: %v", len(transitions))
	}

	// both transitions should have the same time since we are using a static time provider
	for i := range transitions {
		if transitions[i].Timestamp.Equal(staticTime) == false {
			t.Errorf("Transitions() returned an unexpected timestamp: %v", transitions[i].Timestamp)
		}
	}
}

func Benchmark_singleTransition(b *testing.B) {
	// CustomEntity represents a custom entity with its current state
	type CustomEntity struct {
		State CustomStateEnum
	}

	entity := &CustomEntity{State: CustomStateEnumA}

	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	var err error

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		entity.State, err = fsm.Transition(CustomStateEnumB, nil)
		if err != nil {
			b.Errorf("Transition returned an error: %v", err)
		}
		fsm.currentState = CustomStateEnumA
	}
}

func Benchmark_twoTransitions(b *testing.B) {
	// CustomEntity represents a custom entity with its current state
	type CustomEntity struct {
		State CustomStateEnum
	}

	entity := &CustomEntity{State: CustomStateEnumA}

	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	tests := []struct {
		targetState CustomStateEnum
	}{
		{CustomStateEnumB},
		{CustomStateEnumA},
	}

	var err error

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, test := range tests {
			entity.State, err = fsm.Transition(test.targetState, nil)
			if err != nil {
				b.Errorf("Transition returned an error: %v", err)
			}
		}
	}
}

func Benchmark_accessCurrentState(b *testing.B) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fsm.CurrentState()
	}
}

func Benchmark_accessTransitions(b *testing.B) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	fsm.Transition(CustomStateEnumB, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fsm.Transitions()
	}
}

func Benchmark_accessTransitionsConcurrently(b *testing.B) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	fsm.Transition(CustomStateEnumB, nil)

	wg := sync.WaitGroup{}
	wg.Add(b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		go func() {
			defer wg.Done()

			_ = fsm.Transitions()
		}()
	}

	wg.Wait()
}

func Benchmark_canTransitionConcurrently(b *testing.B) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	fsm.Transition(CustomStateEnumB, nil)

	wg := sync.WaitGroup{}
	wg.Add(b.N)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		go func() {
			defer wg.Done()

			_ = fsm.CanTransition(CustomStateEnumA)
		}()
	}

	wg.Wait()
}

func Benchmark_marshalJSON(b *testing.B) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	fsm.Transition(CustomStateEnumB, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(fsm)
	}
}

func Benchmark_unmarshalJSON(b *testing.B) {
	// Create a sample FSM JSON data
	jsonData := []byte(`{
		"current_state": "stateB",
		"transitions": [
			{
				"from_state": "stateA",
				"to_state": "stateB",
				"timestamp": "2022-01-01T12:00:00Z",
				"metadata": {
					"reason": "Transition from stateA to stateB"
				}
			}
		]
	}`)

	// Create an FSM instance to test
	fsm := &FSM[string]{
		currentState: "initial",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Unmarshal the JSON data into the FSM
		err := json.Unmarshal(jsonData, &fsm)
		if err != nil {
			b.Errorf("UnmarshalJSON failed: %v", err)
		}
	}
}

func Benchmark_String(b *testing.B) {
	fsm := NewFSM[CustomStateEnum](CustomStateEnumA, 10)
	fsm.AddRule(CustomStateEnumA, CustomStateEnumB)
	fsm.AddRule(CustomStateEnumB, CustomStateEnumA)

	fsm.Transition(CustomStateEnumB, nil)
	fsm.Transition(CustomStateEnumA, nil)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = fsm.String()
	}
}
