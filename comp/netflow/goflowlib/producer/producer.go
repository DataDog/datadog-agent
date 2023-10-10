package producer

import (
	"fmt"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/netsampler/goflow2/v2/decoders/netflow"
	"github.com/netsampler/goflow2/v2/decoders/netflowlegacy"
	"github.com/netsampler/goflow2/v2/decoders/sflow"
	"github.com/netsampler/goflow2/v2/producer"
	"github.com/netsampler/goflow2/v2/producer/proto"
	"sync"
)

type Producer struct {
	cfgMapped          *configMapped
	namespace          string
	flowAggIn          chan *common.Flow
	samplinglock       *sync.RWMutex
	sampling           map[string]protoproducer.SamplingRateSystem
	samplingRateSystem func() protoproducer.SamplingRateSystem
}

func (p *Producer) getSamplingRateSystem(args *producer.ProduceArgs) protoproducer.SamplingRateSystem {
	key := args.Src.Addr().String()
	p.samplinglock.RLock()
	sampling, ok := p.sampling[key]
	p.samplinglock.RUnlock()
	if !ok {
		sampling = p.samplingRateSystem()
		p.samplinglock.Lock()
		p.sampling[key] = sampling
		p.samplinglock.Unlock()
	}

	return sampling
}

func (p *Producer) Produce(msg interface{}, args *producer.ProduceArgs) ([]producer.ProducerMessage, error) {
	exporterAddress, err := args.SamplerAddress.MarshalBinary()

	if err != nil {
		return nil, fmt.Errorf("invalid exporter address")
	}

	var flows []*common.Flow

	switch msgConv := msg.(type) {
	case *netflowlegacy.PacketNetFlowV5:
		flows, err = ProcessMessageNetFlowLegacy(msgConv, exporterAddress)
	case *netflow.NFv9Packet, *netflow.IPFIXPacket:
		samplingRateSystem := p.getSamplingRateSystem(args)
		flows, err = ProcessMessageNetFlowConfig(msgConv, samplingRateSystem, p.cfgMapped, exporterAddress)
	case *sflow.Packet:
		flows, err = ProcessMessageSFlowConfig(msgConv, p.cfgMapped, exporterAddress)
	default:
		return nil, fmt.Errorf("flow not recognized")
	}

	var producedFlows []producer.ProducerMessage

	for _, flow := range flows {
		producedFlows = append(producedFlows, flow)
	}

	return producedFlows, err
}

func (p *Producer) Commit(flows []producer.ProducerMessage) {
	for _, flow := range flows {
		f := flow.(*common.Flow)
		f.Namespace = p.namespace
		p.flowAggIn <- f
	}
}

func (p *Producer) Close() {
}

func CreateProducer(cfg *config.Mapping, namespace string, flowAggIn chan *common.Flow) *Producer {
	cfgMapped := mapConfig(cfg)
	return &Producer{
		cfgMapped:          cfgMapped,
		namespace:          namespace,
		flowAggIn:          flowAggIn,
		samplinglock:       &sync.RWMutex{},
		sampling:           make(map[string]protoproducer.SamplingRateSystem),
		samplingRateSystem: protoproducer.CreateSamplingSystem,
	}
}
