package model

import "testing"

func TestGraph(t *testing.T) {
	edges := []TaskDependency{
		{TaskID: 2, DependsOnTaskID: 1},
		{TaskID: 3, DependsOnTaskID: 1},
		{TaskID: 4, DependsOnTaskID: 2},
	}
	g := Graph(edges)
	if len(g) != 3 {
		t.Fatalf("expected 3 nodes in graph, got %d", len(g))
	}
	if len(g[2]) != 1 || g[2][0] != 1 {
		t.Fatal("task 2 should depend on task 1")
	}
}

func TestDetectCycleNoCycle(t *testing.T) {
	edges := []TaskDependency{
		{TaskID: 2, DependsOnTaskID: 1},
		{TaskID: 3, DependsOnTaskID: 1},
	}
	g := Graph(edges)
	if DetectCycle(g) {
		t.Fatal("expected no cycle")
	}
}

func TestDetectCycleWithCycle(t *testing.T) {
	edges := []TaskDependency{
		{TaskID: 1, DependsOnTaskID: 2},
		{TaskID: 2, DependsOnTaskID: 3},
		{TaskID: 3, DependsOnTaskID: 1},
	}
	g := Graph(edges)
	if !DetectCycle(g) {
		t.Fatal("expected cycle")
	}
}

func TestDetectCycleSelfLoop(t *testing.T) {
	edges := []TaskDependency{
		{TaskID: 1, DependsOnTaskID: 1},
	}
	g := Graph(edges)
	if !DetectCycle(g) {
		t.Fatal("expected cycle for self-loop")
	}
}

func TestDetectCycleEmpty(t *testing.T) {
	if DetectCycle(Graph(nil)) {
		t.Fatal("expected no cycle for empty graph")
	}
}

func TestAllDependenciesSatisfied(t *testing.T) {
	edges := []TaskDependency{
		{TaskID: 3, DependsOnTaskID: 1},
		{TaskID: 3, DependsOnTaskID: 2},
	}
	g := Graph(edges)

	if AllDependenciesSatisfied(g, 3, map[int64]bool{1: true}) {
		t.Fatal("should not be satisfied when dep 2 is missing")
	}
	if !AllDependenciesSatisfied(g, 3, map[int64]bool{1: true, 2: true}) {
		t.Fatal("should be satisfied when all deps are present")
	}
	if !AllDependenciesSatisfied(g, 5, nil) {
		t.Fatal("task with no deps should be satisfied")
	}
}
