package topograph

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/snmp/internal/topopayload"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/goccy/go-graphviz"
	"github.com/goccy/go-graphviz/cgraph"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// GraphTopology TODO
func GraphTopology() {
	connections := make(map[string]struct{})

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
		log.Debugf("file: %s", file)
		graphForFile(graph, file, connections)
	}

	renderGraph(g, graph)
}

func findFiles(root, ext string) []string {
	var a []string
	_ = filepath.WalkDir(root, func(s string, d fs.DirEntry, e error) error {
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

func graphForFile(graph *cgraph.Graph, sourceFile string, connections map[string]struct{}) {
	jsonFile, err := os.Open(sourceFile)
	// if we os.Open returns an error then handle it
	if err != nil {
		log.Error(err)
		return
	}
	defer jsonFile.Close()

	profile := filepath.Base(sourceFile)

	byteValue, _ := ioutil.ReadAll(jsonFile)

	payload := topopayload.TopologyPayload{}
	err = json.Unmarshal(byteValue, &payload)

	if err != nil {
		log.Error(err)
	}

	log.Debugf("payload: %+v", payload)

	payload.Device.Name = payload.Device.Name + "(" + profile + ")" // TODO: refactor me, this is a workaround to create a device per profile

	localDev, err := createNode(graph, payload.Device)
	localDev.SetColor("red")
	if err != nil {
		log.Error(err)
		return
	}
	for _, conn := range payload.Connections {
		device := conn.Remote.Device

		remDev, err := createNode(graph, device)
		if err != nil {
			log.Error(err)
			continue
		}

		headIf := interfaceName(conn.Local.Interface)
		tailIf := interfaceName(conn.Remote.Interface)

		if localDev.Name() == remDev.Name() {
			continue
		}

		idElems := []string{remDev.Name(), localDev.Name(), headIf, tailIf}
		sort.Strings(idElems)
		edgeID := strings.Join(idElems, "-")
		if _, ok := connections[edgeID]; ok {
			continue
		}
		connections[edgeID] = struct{}{}

		e, err := graph.CreateEdge("", remDev, localDev)
		if err != nil {
			log.Error(err)
			return
		}

		e.SetHeadLabel("\n\n" + headIf + "\n\n")
		e.SetTailLabel("\n\n" + tailIf + "\n\n")
		e.SetArrowHead(cgraph.NoneArrow)
	}
}

func interfaceName(interf topopayload.Interface) string {
	formatInterfaceName := interf.ID
	//if interf.IDType != "" {
	//	formatInterfaceName += "\n(" + interf.IDType + ")"
	//}
	return formatInterfaceName
}

func createNode(graph *cgraph.Graph, device topopayload.Device) (*cgraph.Node, error) {
	var nodeName string
	//var nodeDesc string
	if device.IP != "" {
		nodeName = device.IP
	}
	//if device.Name != "" {
	//	if nodeDesc != "" {
	//		nodeDesc += "\n"
	//	}
	//	nodeDesc += "Name: " + device.Name
	//}
	//if device.ChassisID != "" {
	//	if nodeDesc != "" {
	//		nodeDesc += "\n"
	//	}
	//	nodeDesc += "chassisId: " + device.ChassisID
	//	if device.ChassisIDType != "" {
	//		nodeDesc += "\nchassisIdType: " + device.ChassisIDType
	//	}
	//}

	if nodeName == "" {
		nodeName = device.ChassisID
	}

	if nodeName == "" {
		return nil, fmt.Errorf("no node name for device: %+v", device)
	}
	remDev, err := graph.CreateNode(nodeName)
	//remDev.SetLabel(nodeDesc)
	if err != nil {
		return nil, err
	}
	return remDev, nil
}

func renderGraph(g *graphviz.Graphviz, graph *cgraph.Graph) {
	defer func() {
		// TODO: FIX PANIC
		if r := recover(); r != nil {
			log.Errorf("Recovered in renderGraph: %v", r)
		}
	}()

	// create your graph

	// 1. write encoded PNG data to buffer
	var buf bytes.Buffer
	if err := g.Render(graph, graphviz.PNG, &buf); err != nil {
		log.Error(err)
		return
	}

	graphFile := "/tmp/topology/graph.png"
	file, err := os.Create(graphFile)
	if err != nil {
		log.Error(err)
		return
	}
	defer file.Close()

	// 3. write to file directly
	if err := g.RenderFilename(graph, graphviz.PNG, graphFile); err != nil {
		log.Error(err)
		return
	}

	log.Debugf("Graph rendered to: %s", graphFile)
}
