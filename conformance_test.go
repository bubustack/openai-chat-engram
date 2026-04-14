package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/bubustack/bubu-sdk-go/conformance"
	"github.com/bubustack/openai-chat-engram/pkg/config"
	"github.com/bubustack/openai-chat-engram/pkg/engram"
)

func TestConformance(t *testing.T) {
	suite := conformance.BatchSuite[config.Config, engram.Input]{
		Engram:      engram.New(),
		Config:      config.Config{},
		Inputs:      engram.Input{},
		ExpectError: true,
		ValidateError: func(err error) error {
			if err == nil {
				return errors.New("expected init error, got nil")
			}
			if !strings.Contains(err.Error(), "OPENAI_API_KEY") || !strings.Contains(err.Error(), "required") {
				return fmt.Errorf("unexpected conformance error: %v", err)
			}
			return nil
		},
	}
	suite.Run(t)
}
