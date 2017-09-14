package main

import (
	"fmt"
	"io"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

func getTracer(name string) (*opentracing.Tracer, io.Closer, error) {
	defaultSamplingServerURL := "http://localhost:5778/sampling"

	cfg := jaegercfg.Configuration{
		Sampler: &jaegercfg.SamplerConfig{
			SamplingServerURL: defaultSamplingServerURL,
			Type:              jaeger.SamplerTypeConst,
			Param:             1,
		},
		Reporter: &jaegercfg.ReporterConfig{
			LogSpans: true,
		},
	}

	// Initialize tracer with a logger and a metrics factory
	tracer, closer, err := cfg.New(name)
	if err != nil {
		return nil, nil, fmt.Errorf("getTracer err: %v", err.Error())
	}
	return &tracer, closer, nil
}
