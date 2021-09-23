package bramble

import (
	"encoding/json"
	"fmt"

	project "github.com/maxmcd/bramble/pkg/brambleproject"
)

func printBuildInput(args []string) error {
	p, err := project.NewProject(".")
	if err != nil {
		return err
	}

	output, err := p.ExecModule(project.ExecModuleInput{
		Command:   "print-build-args",
		Arguments: args,
	})
	if err != nil {
		return err
	}
	b, err := json.Marshal(output)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}
