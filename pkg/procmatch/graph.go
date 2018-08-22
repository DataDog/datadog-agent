package procmatch

import (
	"fmt"
	"strings"
	"unicode"
)

// signatureGraph holds the catalog as a graph to be able to quickly find an integration for a given command line (split by word)
// - Nodes either represent an integration, or an intermediary state that may lead to another, more specific integration
// - Edges represent the string components of the signature, which ultimately map to an integration
// e.g.
// (root) ---"java"---> (integration: java) ---"elasticsearch"-> (integration: elastic search)
//                                      \--"kafka.Kafka"---> (integration: kafka)
//                                      \--"jar"-----------> (integration: nil) ---"MapReduce.jar"---> (integration: map reduce)
//                                                                          \--"zookeeper.jar"---> (integration: zookeeper)
type signatureGraph struct {
	root *node
}

// Node represents a node on the signatureGraph
type node struct {
	// The integration field indicates if this node actually represents a valid integration.
	// While walking through the graph, if we reach a node whose integration != "", it means that we found
	// a potential integration to the cmdline
	integration string

	// We use a map to link a node to its children so at which time that we try to move on the graph
	// we can do it in a O(1) operation
	children map[string]*node
}

// buildsignatureGraph takes a slice of signatures and constructs a signatureGraph for them
func buildSignatureGraph(signatures []signature) (*signatureGraph, error) {
	graph := &signatureGraph{
		root: &node{
			integration: "",
			children:    make(map[string]*node),
		},
	}

	// Recursively construct the graph from the root
	if err := expandNode(graph.root, signatures); err != nil {
		return nil, err
	}
	return graph, nil
}

// expandNode analyzes the signatures of a node and groups the signatures with same first word.
// Then it creates a new child node for each group and recursively applies the same process for each child
func expandNode(n *node, sigs []signature) error {
	if len(sigs) == 0 { // It's a leaf node so nothing left to be done
		return nil
	}

	signaturesForNode := make(map[*node][]signature)

	// Searches for signatures with same first word, creates a new node for them and remove them from the sigs list
	for len(sigs) > 0 {
		sig := sigs[0]
		word := sig.words[0]

		newNode := &node{
			integration: "",
			children:    make(map[string]*node),
		}
		n.children[word] = newNode

		// If the signature has more words to be processed, the remaining part of it must be in the child's signatures.
		// Otherwise, this signature has been all processed and the child'll contain its integration
		if len(sig.words) > 1 {
			sig.words = sig.words[1:]
			signaturesForNode[newNode] = append(signaturesForNode[newNode], sig)
		} else {
			newNode.integration = sig.integration
		}

		// Stores the position of the signatures with the same first word to remove them from the sigs list
		toRemove := make([]int, 0)

		// Searches for signatures with same path
		for i := range sigs {
			s := sigs[i]

			// This signature has the same path at the current one
			if s.words[0] == word {
				toRemove = append(toRemove, i)

				if len(s.words) == 1 && newNode.integration != "" && s.integration != newNode.integration {
					return fmt.Errorf("Two different signatures are leading to the same node with diferent integration."+
						"Current :%s with %s + %s, other %s with %s\n", sig.integration, word, sig.words, s.integration, s.words)
				}

				// Verify if this signature has been completely processed
				if len(s.words) > 1 {
					s.words = s.words[1:]
					signaturesForNode[newNode] = append(signaturesForNode[newNode], s)
				} else {
					newNode.integration = s.integration
				}
			}
		}

		// Create a new filtered sigs list
		sigs = removeByIndex(sigs, toRemove)
	}

	// Apply the same process recursively on each child
	for _, child := range n.children {
		err := expandNode(child, signaturesForNode[child])
		if err != nil {
			return err
		}
	}
	return nil
}

func removeByIndex(sigs []signature, indices []int) []signature {
	filtered := []signature{}
	for i := range sigs {
		if len(indices) > 0 && i == indices[0] {
			indices = indices[1:]
			continue
		}
		filtered = append(filtered, sigs[i])
	}
	return filtered
}

// splitCmdline splits a cmdline by " " or "/"
func splitCmdline(r rune) bool {
	return r == '/' || unicode.IsSpace(r)
}

func (g *signatureGraph) searchIntegration(cmdline string) string {
	integration := ""
	walk(g.root, strings.FieldsFunc(strings.ToLower(cmdline), splitCmdline), &integration)
	return integration
}

// walk takes the words from a cmdline and use them to move trough the graph
func walk(node *node, cmdline []string, integration *string) {
	// If we reach a node with an integration, it's potentially the cmdline integration
	if node.integration != "" {
		*integration = node.integration
	}

	if len(node.children) == 0 || len(cmdline) == 0 {
		return
	}

	// Each one of the words of a cmdline is used to walk through the graph
	for i := range cmdline {
		// If this word links the current node to a child, we move to the child
		if nextNode, ok := node.children[cmdline[i]]; ok {
			walk(nextNode, cmdline[i+1:], integration)
			break
		}
	}
}

func bfs(n *node) {
	queue := make([]*node, 0)
	queue = append(queue, n)

	for len(queue) > 0 {
		actual := queue[0]
		fmt.Printf("Node: %s \n", actual.integration)

		queue = queue[1:]

		for word, child := range actual.children {
			fmt.Printf("\t%s -> %s\n", word, child.integration)
			queue = append(queue, child)
		}
	}
}
