package runner

import "github.com/projectdiscovery/gologger"

const banner = `
   ________                _____      __ 
  / ____/ /___  __  ______/ / (_)____/ /_
 / /   / / __ \/ / / / __  / / / ___/ __/
/ /___/ / /_/ / /_/ / /_/ / / (__  ) /_  
\____/_/\____/\__,_/\__,_/_/_/____/\__/  v1.0.1								  
`

// Version is the current version of nuclei
const Version = `1.0.1`

// showBanner is used to show the banner to the user
func showBanner() {
	gologger.Print().Msgf("%s\n", banner)
	gologger.Print().Msgf("\t\tprojectdiscovery.io\n\n")

	gologger.Print().Msgf("Use with caution. You are responsible for your actions\n")
	gologger.Print().Msgf("Developers assume no liability and are not responsible for any misuse or damage.\n")
}
