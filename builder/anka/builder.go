package anka

import (
	"context"
	"github.com/hashicorp/hcl/v2/hcldec"
	"log"

	"github.com/hashicorp/packer/common"
	"github.com/hashicorp/packer/helper/communicator"
	"github.com/hashicorp/packer/helper/multistep"
	"github.com/hashicorp/packer/packer"
	"github.com/veertuinc/packer-builder-veertu-anka/client"
)

// The unique ID for this builder.
const BuilderId = "packer.veertu-anka"

// Builder represents a Packer Builder.
type Builder struct {
	config *Config
	runner multistep.Runner
}

// Prepare processes the build configuration parameters.
func (b *Builder) Prepare(raws ...interface{}) (params []string, warnings []string, retErr error) {
	c, errs := NewConfig(raws...)
	if errs != nil {
		return nil, nil, errs
	}
	b.config = c
	return nil, nil, nil
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

// Run executes an Anka Packer build and returns a packer.Artifact
func (b *Builder) Run(ctx context.Context, ui packer.Ui, hook packer.Hook) (packer.Artifact, error) {
	client := &client.Client{}

	version, err := client.Version()
	if err != nil {
		return nil, err
	}
	log.Printf("[DEBUG] Anka version: %s", version)

	steps := []multistep.Step{
		&StepTempDir{},
		&StepCreateVM{},
		&StepSetHyperThreading{},
		&StepStartVM{},
		&communicator.StepConnect{
			Config: &b.config.Comm,
			CustomConnect: map[string]multistep.Step{
				"anka": &StepConnectAnka{},
			},
		},
		&common.StepProvision{},
	}

	// Setup the state bag and initial state for the steps
	state := new(multistep.BasicStateBag)
	state.Put("config", b.config)
	state.Put("hook", hook)
	state.Put("ui", ui)
	state.Put("client", client)

	// Run!
	b.runner = common.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	// If there was an error, return that
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// If it was cancelled, then just return
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, nil
	}

	// Check we can describe the VM
	descr, err := client.Describe(state.Get("vm_name").(string))
	if err != nil {
		return nil, err
	}

	// No errors, must've worked
	return &Artifact{
		vmId:   descr.UUID,
		vmName: descr.Name,
	}, nil
}
