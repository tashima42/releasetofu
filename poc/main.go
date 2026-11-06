package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	"go.yaml.in/yaml/v4"
)

const delimiter = ">"

type ActionData struct {
	Action    string      `yaml:"action"`
	Plugin    string      `yaml:"plugin"`
	Data      map[any]any `yaml:"data"`
	DependsOn []string    `yaml:"depends_on"`
}

type Config struct {
	ID       string    `yaml:"id"`
	Version  int       `yaml:"version"`
	Releases yaml.Node `yaml:"releases"`
}

var (
	SymbolTable = make(map[string]ActionData)
	graph       = make(map[string][]string)
	inDegree    = make(map[string]int)
)

func main() {
	fileb, err := os.ReadFile("../example_config.yml")
	if err != nil {
		log.Fatal(err)
	}
	var config Config
	if err := yaml.Load(fileb, &config, yaml.WithKnownFields(false)); err != nil {
		log.Fatal(err)
	}

	fmt.Println("\n--- config output---")
	fmt.Printf("id: %s\nversion: %d\n", config.ID, config.Version)

	if err := walkDFS(&config.Releases, []string{}, []string{}); err != nil {
		log.Fatal(err)
	}

	expandDependencies()
	populateDAG()

	execOrder := kahnsAlgorithm()

	for _, task := range execOrder {
		fmt.Println(task)
	}

	// for key, value := range inDegree {
	// 	fmt.Printf("fqn: %s: %v\n", key, value)
	// }
}

func walkDFS(node *yaml.Node, pathStack []string, inheritedDeps []string) error {
	if node == nil {
		return nil
	}

	deps := extractDependsOn(node)

	currentDeps := append(inheritedDeps, deps...)

	if isActionable(node) {
		var actionData ActionData

		if err := node.Decode(&actionData); err != nil {
			return err
		}

		fqn := joinSequence(pathStack)
		actionData.DependsOn = currentDeps
		SymbolTable[fqn] = actionData
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valueNode := node.Content[i+1]

		// Clone the pathStack to prevent branch collisions
		newPath := append([]string{}, pathStack...)
		// Ignore "steps" because it is only used to identify children
		if keyNode.Value != "steps" {
			newPath = append(newPath, keyNode.Value)
		}

		if err := walkDFS(valueNode, newPath, currentDeps); err != nil {
			return err
		}
	}
	return nil
}

// isActionable checks if a node has the 'action' value, if it has, it's a leaf node and doesn't contain any children
func isActionable(node *yaml.Node) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]

		if keyNode.Value == "action" {
			return true
		}
	}
	return false
}

func joinSequence(stack []string) string {
	return strings.Join(stack, delimiter)
}

func extractDependsOn(node *yaml.Node) []string {
	deps := []string{}

	if node == nil || node.Kind != yaml.MappingNode {
		return deps
	}

	for i := 0; i < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		if keyNode.Value == "depends_on" {
			valueNode := node.Content[i+1]
			if valueNode.Kind == yaml.SequenceNode {
				for _, item := range valueNode.Content {
					deps = append(deps, item.Value)
				}
			}
			break
		}
	}

	return deps
}

func expandDependencies() {
	for fqn, payload := range SymbolTable {
		var expandedDeps []string

		for _, rawDep := range payload.DependsOn {
			targetPath := resolvePath(fqn, rawDep)

			if _, exists := SymbolTable[targetPath]; exists {
				expandedDeps = append(expandedDeps, targetPath)
				continue
			}
			prefix := targetPath + ">"
			matchedGroup := false

			for tableKey := range SymbolTable {
				if strings.HasPrefix(tableKey, prefix) {
					expandedDeps = append(expandedDeps, tableKey)
					matchedGroup = true
				}
			}
			if !matchedGroup {
				// Handle error: dependency not found as leaf or group
			}
		}
		payload.DependsOn = expandedDeps
		SymbolTable[fqn] = payload
	}
}

func resolvePath(fqn string, rawDep string) string {
	parts := splitFQN(fqn)
	parentPath := parts[:len(parts)-1]

	for i := len(parts); i >= 0; i-- {
		candidate := rawDep

		if i != 0 {
			scope := joinSequence(parentPath[:i])
			candidate = scope + delimiter + rawDep
		}

		if isValidTarget(candidate) {
			return candidate
		}
	}
	return rawDep
}

func isValidTarget(target string) bool {
	if _, isFullFQN := SymbolTable[target]; isFullFQN {
		return true
	}

	prefix := target + delimiter

	for fqn := range SymbolTable {
		if strings.HasPrefix(fqn, prefix) {
			return true
		}
	}
	return false
}

func splitFQN(fqn string) []string {
	return strings.Split(fqn, delimiter)
}

func populateDAG() {
	for fqn := range SymbolTable {
		graph[fqn] = []string{}
		inDegree[fqn] = 0
	}

	for fqn, payload := range SymbolTable {
		for _, dep := range payload.DependsOn {
			graph[dep] = append(graph[dep], fqn)

			inDegree[fqn]++
		}
	}
}

func kahnsAlgorithm() []string {
	var queue []string
	var executionOrder []string

	// 1. Find all nodes with 0 in-degree
	for fqn, count := range inDegree {
		if count == 0 {
			queue = append(queue, fqn)
		}
	}
	// 2. Process nodes
	for len(queue) > 0 {
		// Dequeue the first node (FIFO)
		u := queue[0]
		queue = queue[1:]

		// Add it to our successful execution order
		executionOrder = append(executionOrder, u)

		// Unblock its dependent neighbors (V)
		for _, v := range graph[u] {
			inDegree[v]-- // Decrement pending dependency count

			// If V has no more pending dependencies, it is ready to run
			if inDegree[v] == 0 {
				queue = append(queue, v)
			}
		}
	}

	return executionOrder
}
