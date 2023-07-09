/*
Package statetrooper provides a finite state machine (FSM) implementation for managing states.

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
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Transition represents information about a state transition
type Transition[T comparable] struct {
	FromState T                 `json:"from_state"`
	ToState   T                 `json:"to_state"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata"`
}

// FSMOption is a function that sets an option on the FSM
type FSMOption[T comparable] func(*FSM[T])

// FSM represents the finite state machine for managing states
type FSM[T comparable] struct {
	currentState T
	transitions  []Transition[T]
	ruleset      map[T][]T
	mu           sync.Mutex
	maxHistory   int

	// timeProvider is used to provide the current time for transitions DEFAULT: time.Now
	timeProvider func() time.Time
}

// NewFSM creates a new instance of FSM with predefined transitions
func NewFSM[T comparable](initialState T, maxHistory int, opts ...FSMOption[T]) *FSM[T] {
	fsm := FSM[T]{
		currentState: initialState,
		ruleset:      make(map[T][]T),
		maxHistory:   maxHistory,
	}

	for _, opt := range opts {
		opt(&fsm)
	}

	fsm.setDefaults()

	return &fsm
}

// WithTimeProvider sets the time provider for the FSM
// The time provider is used to provide the current time for transitions
// DEFAULT: time.Now
// This is useful for testing or when you want to set a time with a specific timezone
func WithTimeProvider[T comparable](provider func() time.Time) FSMOption[T] {
	return func(fsm *FSM[T]) {
		fsm.timeProvider = provider
	}
}

// CanTransition checks if a transition from the current state to the target state is valid
func (fsm *FSM[T]) CanTransition(targetState T) bool {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	return fsm.canTransition(&fsm.currentState, &targetState)
}

// canTransition checks if a transition from one state to another state is valid
func (fsm *FSM[T]) canTransition(fromState *T, toState *T) bool {
	validTransitions, ok := fsm.ruleset[*fromState]
	if !ok {
		return false
	}

	for _, validState := range validTransitions {
		if validState == *toState {
			return true
		}
	}

	return false
}

// AddRule adds a valid transition between two states
func (fsm *FSM[T]) AddRule(fromState T, toState ...T) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	fsm.ruleset[fromState] = append(fsm.ruleset[fromState], toState...)
}

// Transition transitions the entity from the current state to the target state
// if the transition is invalid, an error is returned and the current state is not changed
func (fsm *FSM[T]) Transition(targetState T, metadata map[string]string) (T, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	if !fsm.canTransition(&fsm.currentState, &targetState) {
		return fsm.currentState, TransitionError[T]{
			FromState: fsm.currentState,
			ToState:   targetState,
		}
	}

	if fsm.maxHistory == 0 {
		fsm.currentState = targetState
		return fsm.currentState, nil
	}

	// Track the transition
	// Check if we need to remove the oldest transition
	if len(fsm.transitions) >= fsm.maxHistory {
		fsm.transitions = fsm.transitions[1:]
	}

	tn := fsm.timeProvider()

	fsm.transitions = append(
		fsm.transitions,
		Transition[T]{
			FromState: fsm.currentState,
			ToState:   targetState,
			Timestamp: tn,
			Metadata:  metadata,
		})

	fsm.currentState = targetState

	return fsm.currentState, nil
}

// CurrentState returns the current state of the FSM
func (fsm *FSM[T]) CurrentState() T {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	return fsm.currentState
}

// Transitions returns a slice of all transitions
func (fsm *FSM[T]) Transitions() []Transition[T] {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	// return a copy of the transitions
	transitions := make([]Transition[T], len(fsm.transitions))

	copy(transitions, fsm.transitions)

	return transitions
}

// GenerateMermaidRulesDiagram generates a Mermaid.js diagram from the FSM's rules
// In order to generate a diagram, T must be a string or have a String() method
func (fsm *FSM[T]) GenerateMermaidRulesDiagram() (string, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	if fsm.ruleset == nil {
		return "", fmt.Errorf("no ruleset defined")
	}

	if len(fsm.ruleset) == 0 {
		return "", fmt.Errorf("no rules defined")
	}

	// Check if T as represented by currentState has a String() method
	if !stringable(fsm.currentState) {
		return "", fmt.Errorf("type T is not a string or does not have a String() method")
	}

	diagram := "graph LR;\n"

	// Nodes for each state
	nodes := make([]string, 0, len(fsm.ruleset))

	for state := range fsm.ruleset {
		nodes = append(nodes, toString(state))
	}

	// Sort nodes
	sort.Strings(nodes)

	// Edges for transitions
	var edges []string

	for fromState, toStates := range fsm.ruleset {
		for _, toState := range toStates {
			edges = append(edges, fmt.Sprintf("%s --> %s;\n", toString(fromState), toString(toState)))
		}
	}

	sort.Strings(edges)

	diagram += strings.Join(nodes, "\n")
	diagram += "\n"
	diagram += strings.Join(edges, "")

	return diagram, nil
}

// GenerateMermaidTransitionHistoryDiagram generates a Mermaid.js diagram from the FSM's transition history
// In order to generate a diagram, the type T must be a string or have a String() method
func (fsm *FSM[T]) GenerateMermaidTransitionHistoryDiagram() (string, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	if fsm.transitions == nil {
		return "", fmt.Errorf("no transition history")
	}

	if len(fsm.transitions) == 0 {
		return "", fmt.Errorf("no transition history")
	}

	// Check if T as represented by currentState has a String() method
	if !stringable(fsm.currentState) {
		return "", fmt.Errorf("type T is not a string or does not have a String() method")
	}

	diagram := "graph TD;\n"

	// Add nodes for each unique state in the transition history
	uniqueStates := make(map[T]bool)
	for _, transition := range fsm.transitions {
		fromState := transition.FromState
		toState := transition.ToState

		uniqueStates[fromState] = true
		uniqueStates[toState] = true
	}

	nodes := make([]string, 0, len(uniqueStates))

	for state := range uniqueStates {
		nodes = append(nodes, fmt.Sprintf("%s;\n", toString(state)))
	}

	// Sort nodes
	sort.Strings(nodes)

	// Add edges with transition order numbers

	edges := make([]string, 0, len(fsm.transitions))

	for i, transition := range fsm.transitions {
		fromState := transition.FromState
		toState := transition.ToState
		transitionNum := i + 1

		edges = append(edges, fmt.Sprintf("%s -->|%d| %s;\n", toString(fromState), transitionNum, toString(toState)))
	}

	sort.Strings(edges)

	diagram += strings.Join(nodes, "")
	diagram += "\n"
	diagram += strings.Join(edges, "")

	return diagram, nil
}

// MarshalJSON serializes the FSM to JSON
func (fsm *FSM[T]) MarshalJSON() ([]byte, error) {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	type FSMExport struct {
		CurrentState T               `json:"current_state"`
		Transitions  []Transition[T] `json:"transitions"`
	}

	export := FSMExport{
		CurrentState: fsm.currentState,
		Transitions:  fsm.transitions,
	}

	return json.Marshal(export)
}

// UnmarshalJSON deserializes the FSM from JSON
func (fsm *FSM[T]) UnmarshalJSON(data []byte) error {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	type FSMImport struct {
		CurrentState T               `json:"current_state"`
		Transitions  []Transition[T] `json:"transitions"`
	}

	var importData FSMImport
	err := json.Unmarshal(data, &importData)
	if err != nil {
		return err
	}

	fsm.currentState = importData.CurrentState

	var s int

	if len(importData.Transitions) < fsm.maxHistory {
		s = len(importData.Transitions)
	} else {
		s = fsm.maxHistory
	}

	fsm.transitions = importData.Transitions[:s]

	return nil
}

// String returns a string representation of the FSM
func (fsm *FSM[T]) String() string {
	fsm.mu.Lock()
	defer fsm.mu.Unlock()

	sb := strings.Builder{}

	sb.WriteString(fmt.Sprintf("Current State: %v\n", fsm.currentState))

	sb.WriteString("Rules:\n")
	for fromState, toStates := range fsm.ruleset {
		sb.WriteString(fmt.Sprintf("\t%v -> %v\n", fromState, toStates))
	}

	sb.WriteString("Transitions:\n")
	for _, transition := range fsm.transitions {
		sb.WriteString(fmt.Sprintf("\t%v\n", transition))
	}

	return sb.String()
}

func (fsm *FSM[T]) setDefaults() {
	if fsm.timeProvider == nil {
		fsm.timeProvider = time.Now
	}
}

// String returns a string representation of the Transition
func (t *Transition[T]) String() string {
	return fmt.Sprintf("Transition from %v to %v at %v with metadata %v", t.FromState, t.ToState, t.Timestamp, t.Metadata)
}
