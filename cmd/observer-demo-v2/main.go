// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"flag"

	observerimpl "github.com/DataDog/datadog-agent/comp/observer/impl"
)

func main() {
	timeScale := flag.Float64("timescale", 0.25, "time multiplier (0.25 = 4x faster)")
	httpAddr := flag.String("http", "", "HTTP address for web dashboard (e.g., :8080)")
	parquetDir := flag.String("parquet", "", "directory containing FGM parquet files for replay")
	loop := flag.Bool("loop", false, "loop parquet replay after reaching the end")

	// Algorithm selection flags (Layer 1 emitters)
	cusum := flag.Bool("cusum", false, "enable CUSUM change-point detector")
	lightESD := flag.Bool("lightesd", false, "enable LightESD statistical outlier detector")
	graphSketch := flag.Bool("graphsketch", false, "enable GraphSketch edge anomaly detector")

	// Correlator selection
	timeClusterCorrelator := flag.Bool("time-cluster", true, "use TimeClusterCorrelator to group anomalies by timestamp proximity")
	graphSketchCorrelator := flag.Bool("graphsketch-correlator", false, "use GraphSketchCorrelator to group anomalies by co-occurrence patterns")

	// Output
	outputFile := flag.String("output", "", "path to write JSON results (anomalies + correlations)")

	// Processing mode
	processAll := flag.Bool("all", false, "process all parquet data without time limit")

	flag.Parse()

	// If no emitters specified, default to CUSUM
	enableCUSUM := *cusum
	if !*cusum && !*lightESD && !*graphSketch {
		enableCUSUM = true
	}

	// Run V2 demo with selected algorithms
	observerimpl.RunDemoV2WithConfig(observerimpl.DemoV2Config{
		TimeScale:                   *timeScale,
		HTTPAddr:                    *httpAddr,
		ParquetDir:                  *parquetDir,
		Loop:                        *loop,
		EnableCUSUM:                 enableCUSUM,
		EnableLightESD:              *lightESD,
		EnableGraphSketch:           *graphSketch,
		UseTimeClusterCorrelator:    *timeClusterCorrelator,
		EnableGraphSketchCorrelator: *graphSketchCorrelator,
		OutputFile:                  *outputFile,
		ProcessAllData:              *processAll,
	})
}
