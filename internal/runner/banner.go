package runner

import (
	"github.com/projectdiscovery/gologger"
	updateutils "github.com/projectdiscovery/utils/update"
)

const banner = `
  _______             _____     __ 
 / ___/ /__  __ _____/ / (_)__ / /_
/ /__/ / _ \/ // / _  / / (_-</ __/
\___/_/\___/\_,_/\_,_/_/_/___/\__/ 
`

// version is the current version of cloudlist
const version = `1.2.2`

// showBanner is used to show the banner to the user
func showBanner() {
	gologger.Print().Msgf("%s\n", banner)
	gologger.Print().Msgf("\t\tprojectdiscovery.io\n\n")
}

// GetUpdateCallback returns a callback function that updates cloudlist
func GetUpdateCallback() func() {
	return func() {
		showBanner()
		updateutils.GetUpdateToolCallback("cloudlist", version)()
	}
}
