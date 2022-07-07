package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/payload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/goccy/go-graphviz/cgraph"
	"io/ioutil"
	"os"

	"github.com/goccy/go-graphviz"
)

// GraphTopology TODO
func GraphTopology() {
	topologyFolder := "/tmp/topology"
	sourceFile := topologyFolder + "/aos6.json"
	jsonFile, err := os.Open(sourceFile)
	defer jsonFile.Close()

	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
	}
	byteValue, _ := ioutil.ReadAll(jsonFile)
	fmt.Println(string(byteValue))

	payload := payload.TopologyPayload{}
	json.Unmarshal(byteValue, &payload)

	log.Infof("payload: %+v", payload)

	g := graphviz.New()
	graph, err := g.Graph()
	if err != nil {
		log.Error(err)
		return
	}
	defer func() {
		if err := graph.Close(); err != nil {
			log.Error(err)
			return
		}
		g.Close()
	}()

	graph.SetOverlap(false)

	localDev, err := createNode(graph, payload.Device)
	if err != nil {
		log.Error(err)
		return
	}
	log.Info(localDev)
	for _, conn := range payload.Connections {
		device := conn.Remote.Device

		remDev, err := createNode(graph, device)
		if err != nil {
			log.Error(err)
			continue
		}

		e, err := graph.CreateEdge("", remDev, localDev)
		if err != nil {
			log.Error(err)
			return
		}
		e.SetHeadLabel(conn.Local.Interface.Id)
		e.SetTailLabel(conn.Remote.Interface.Id)
	}

	renderGraph(g, graph, err)
}

func createNode(graph *cgraph.Graph, device payload.Device) (*cgraph.Node, error) {
	var nodeName string
	if device.IP != "" {
		nodeName = device.IP
	}
	if device.Name != "" {
		if nodeName != "" {
			nodeName += "\n"
		}
		nodeName += "(" + device.Name + ")"
	}
	if nodeName == "" && device.ChassisId != "" {
		if nodeName != "" {
			nodeName += "\n"
		}
		nodeName += "[" + device.ChassisId + "]"
	}

	if nodeName == "" {
		return nil, fmt.Errorf("no node name for device: %+v", device)
	}
	remDev, err := graph.CreateNode(nodeName)
	if err != nil {
		return nil, err
	}
	return remDev, nil
}

func renderGraph(g *graphviz.Graphviz, graph *cgraph.Graph, err error) {
	// create your graph

	// 1. write encoded PNG data to buffer
	var buf bytes.Buffer
	if err := g.Render(graph, graphviz.PNG, &buf); err != nil {
		log.Error(err)
		return
	}

	graphFile := "/tmp/topology/graph.png"
	file, err := os.Create(graphFile)
	defer file.Close()
	if err != nil {
		log.Error(err)
		return
	}

	// 3. write to file directly
	if err := g.RenderFilename(graph, graphviz.PNG, graphFile); err != nil {
		log.Error(err)
		return
	}
}
