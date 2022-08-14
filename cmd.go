/*----------------------------------------------------------------------------------------
 * Copyright (c) Microsoft Corporation. All rights reserved.
 * Licensed under the MIT License. See LICENSE in the project root for license information.
 *---------------------------------------------------------------------------------------*/

package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

const clientID = "5fcb127516083e890182"
const deviceURL = "https://github.com/login/device/code"
const tokenURL = "https://github.com/login/oauth/access_token"

var scope = []string{"gist"}

func init() {
	createConfigDir()
}

func main() {
	model := newModel()
	p := tea.NewProgram(model)
	if err := p.Start(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
	fmt.Println(model.result)
}
