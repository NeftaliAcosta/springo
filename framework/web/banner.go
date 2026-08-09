package web

import (
	"fmt"

	"github.com/NeftaliAcosta/springo/framework/version"
)

const banner = `
  ____             _        ____
 / ___| _ __  _ __(_)_ __  / ___| ___
 \___ \| '_ \| '__| | '_ \| |  _ / _ \
  ___) | |_) | |  | | | | | |_| | (_) |
 |____/| .__/|_|  |_|_| |_|\____|\___/
       |_|
`

// ShowBanner prints the SprinGo startup banner
func ShowBanner() {
	fmt.Print(banner)
	fmt.Printf(" :: %s ::       (%s)\n", version.Name, version.Current)
	fmt.Println(" :: Powered by: NeftaliAcosta ::")
	fmt.Println()
}
