package graph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/networkdiscovery/payload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/goccy/go-graphviz/cgraph"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/goccy/go-graphviz"
)

// GraphTopology TODO
func GraphTopology() {
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
	// layouts: circo dot fdp neato nop nop1 nop2 osage patchwork sfdp twopi
	graph.SetLayout("sfdp")
	topologyFolder := "/tmp/topology"
	for _, file := range findFiles(topologyFolder, ".json") {
		fmt.Println(file)
		graphForFile(graph, file)
	}

	renderGraph(g, graph)
}

func findFiles(root, ext string) []string {
	var a []string
	filepath.WalkDir(root, func(s string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if filepath.Ext(d.Name()) == ext {
			a = append(a, s)
		}
		return nil
	})
	return a
}

func graphForFile(graph *cgraph.Graph, sourceFile string) {
	jsonFile, err := os.Open(sourceFile)
	defer jsonFile.Close()

	profile := filepath.Base(sourceFile)

	// if we os.Open returns an error then handle it
	if err != nil {
		fmt.Println(err)
	}
	byteValue, _ := ioutil.ReadAll(jsonFile)
	fmt.Println(string(byteValue))

	payload := payload.TopologyPayload{}
	json.Unmarshal(byteValue, &payload)

	log.Infof("payload: %+v", payload)

	payload.Device.Name = profile // TODO: refactor me, this is a workaround to create a device per profile

	localDev, err := createNode(graph, payload.Device)
	localDev.SetColor("red")
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

		e.SetHeadLabel("\n\n" + interfaceName(conn.Local.Interface) + "\n\n")
		e.SetTailLabel("\n\n" + interfaceName(conn.Remote.Interface) + "\n\n")
		e.SetArrowHead(cgraph.NoneArrow)
	}
}

func interfaceName(interf payload.Interface) string {
	formatInterfaceName := interf.Id
	if interf.IdType != "" {
		formatInterfaceName += "\n(" + interf.IdType + ")"
	}
	return formatInterfaceName
}

func createNode(graph *cgraph.Graph, device payload.Device) (*cgraph.Node, error) {
	var nodeName string
	if device.IP != "" {
		nodeName = "IP: " + device.IP
	}
	if device.Name != "" {
		if nodeName != "" {
			nodeName += "\n"
		}
		nodeName += "Name: " + device.Name
	}
	if device.ChassisId != "" {
		if nodeName != "" {
			nodeName += "\n"
		}
		nodeName += "chassisId: " + device.ChassisId
		if device.ChassisIdType != "" {
			nodeName += "\nchassisIdType: " + device.ChassisIdType
		}
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

func renderGraph(g *graphviz.Graphviz, graph *cgraph.Graph) {
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
