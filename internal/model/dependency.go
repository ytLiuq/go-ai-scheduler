package model

// TaskDependency represents a directional dependency: TaskID depends on DependsOnTaskID.
type TaskDependency struct {
	ID               int64
	TaskID           int64
	DependsOnTaskID  int64
}

// Graph builds the full dependency graph for a set of edges. Returns an
// adjacency list suitable for topological sort and cycle detection.
func Graph(edges []TaskDependency) map[int64][]int64 {
	graph := make(map[int64][]int64)
	for _, e := range edges {
		graph[e.TaskID] = append(graph[e.TaskID], e.DependsOnTaskID)
	}
	return graph
}

// DetectCycle returns true if the dependency graph contains a cycle.
func DetectCycle(graph map[int64][]int64) bool {
	visited := make(map[int64]int) // 0=unvisited, 1=in-progress, 2=done
	var dfs func(node int64) bool
	dfs = func(node int64) bool {
		visited[node] = 1
		for _, dep := range graph[node] {
			if visited[dep] == 1 {
				return true
			}
			if visited[dep] == 0 {
				if dfs(dep) {
					return true
				}
			}
		}
		visited[node] = 2
		return false
	}
	for node := range graph {
		if visited[node] == 0 {
			if dfs(node) {
				return true
			}
		}
	}
	return false
}

// AllDependenciesSatisfied checks whether all upstream dependencies of taskID
// have a successful instance within the given lookup window.
func AllDependenciesSatisfied(graph map[int64][]int64, taskID int64, successfulInstances map[int64]bool) bool {
	deps := graph[taskID]
	if len(deps) == 0 {
		return true
	}
	for _, dep := range deps {
		if !successfulInstances[dep] {
			return false
		}
	}
	return true
}
