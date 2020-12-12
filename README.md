<h1 align="center">
  <img src="static/cloudlist-logo.png" alt="cloudlist" width="400px"></a>
  <br>
</h1>


[![License](https://img.shields.io/badge/license-MIT-_red.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/projectdiscovery/cloudlist)](https://goreportcard.com/report/github.com/projectdiscovery/cloudlist)
[![contributions welcome](https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat)](https://github.com/projectdiscovery/cloudlist/issues)
[![GitHub Release](https://img.shields.io/github/release/projectdiscovery/cloudlist)](https://github.com/projectdiscovery/cloudlist/releases)
[![Follow on Twitter](https://img.shields.io/twitter/follow/pdiscoveryio.svg?logo=twitter)](https://twitter.com/pdiscoveryio)
[![Chat on Discord](https://img.shields.io/discord/695645237418131507.svg?logo=discord)](https://discord.gg/KECAGdH)

Cloudlist is a multi-cloud tool for getting Assets (Hostnames, IP Addresses) from Cloud Providers. This is intended to be used by the blue team to augment Attack Surface Management efforts by maintaining a centralized list of assets across multiple clouds with very little configuration efforts.

# Resources

- [Features](#features)
- [Usage](#usage)
- [Installation Instructions](#installation-instructions)
    - [From Source](#from-source)
    - [From Source](#from-source-1)
    - [From Github](#from-github)
- [Configuration file](#configuration-file)
- [Running cloudlist](#running-cloudlist)
- [Supported providers](#supported-providers)
- [Todo](#todo)
- [Cloudlist as a library](#cloudlist-as-a-library)
- [Acknowledgments](#acknowledgments)
- [License](#license)

# Features

<h1 align="left">
  <img src="static/cloudlist-run.png" alt="cloudlist" width="700px"></a>
  <br>
</h1>


 - Easily list Cloud assets with multiple configurations.
 - Multiple cloud providers support.
 - Highly extensible making adding new providers a breeze.
 - **STDOUT** support to work with other tools in pipelines.

# Usage

```sh
▶ cloudlist -h
```

This will display help for the tool. Here are all the switches it supports.

| Flag     | Description                    | Example                     |
| -------- | ------------------------------ | --------------------------- |
| config   | Config file for providers      | cloudlist -config test.yaml |
| provider | List assets of given providers | cloudlist -provider aws     |
| host     | List hosts only                | cloudlist -host             |
| ip       | List Ips only                  | cloudlist -ip               |
| json     | List output in the JOSN format | cloudlist -json             |
| output   | Store the output in file       | cloudlist -output           |
| silent   | Display results only           | cloudlist -silent           |
| version  | Display current version        | cloudlist -version          |
| verbose  | Display verbose mode           | cloudlist -verbose          |

# Installation Instructions

### From Source

The installation is easy. You can download the pre-built binaries for your platform from the [Releases](https://github.com/projectdiscovery/cloudlist/releases/) page. Extract them using tar, move it to your `$PATH`and you're ready to go.

```sh
Download latest binary from https://github.com/projectdiscovery/cloudlist/releases

▶ tar -xvf cloudlist-linux-amd64.tar
▶ mv cloudlist-linux-amd64 /usr/local/bin/cloudlist
▶ cloudlist -h
```

### From Source

**cloudlist** requires **go1.14+** to install successfully. Run the following command to get the repo - 

```sh
▶ GO111MODULE=on go get -u -v github.com/projectdiscovery/cloudlist/cmd/cloudlist
```

### From Github

```sh
▶ git clone https://github.com/projectdiscovery/cloudlist.git; cd cloudlist/cmd/cloudlist; go build; cp cloudlist /usr/local/bin/; cloudlist -version
```

# Configuration file

The default config file should be located in `$HOME/.config/cloudlist/config.yaml` and has the following contents as an example. In order to run this tool, the keys need to updated in the config file.

```yaml

# Configuration file for cloudlist enumeration agent
- # provider is the name of the provider (Digitalocean)
  provider: do
  # profile is the name of the provider profile
  profile: xxxx
  # digitalocean_token is the API key for digitalocean cloud platform
  digitalocean_token: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

- # provider is the name of the provider (Scaleway)
  provider: scw
  # scaleway_access_key is the access key for scaleway API
  scaleway_access_key: SCWXXXXXXXXXXXXXX
  # scaleway_access_token is the access token for scaleway API
  scaleway_access_token: xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxx

- # provider is the name of the provider (Amazon Web Services)
  provider: aws
  # profile is the name of the provider profile
  profile: staging
  # aws_access_key is the access key for AWS account
  aws_access_key: AKIAXXXXXXXXXXXXXX
  # aws_secret_key is the secret key for AWS account
  aws_secret_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx

- # provider is the name of the provider (Google Cloud Platform)
  provider: gcp
  # profile is the name of the provider profile
  profile: logs
  # gcp_service_account_key is the minified json of a google cloud service account with list permissions
  gcp_service_account_key: '{xxxxxxxxxxxxx}'
```

# Running cloudlist

```
▶ cloudlist
```

This will list all the assets from configured providers in the configuration file. Specific providers and asset type can also be specified using available flags.

```bash

▶ cloudlist -provider aws

   ________                _____      __ 
  / ____/ /___  __  ______/ / (_)____/ /_
 / /   / / __ \/ / / / __  / / / ___/ __/
/ /___/ / /_/ / /_/ / /_/ / / (__  ) /_  
\____/_/\____/\__,_/\__,_/_/_/____/\__/  v0.0.1        

    projectdiscovery.io

[WRN] Use with caution. You are responsible for your actions
[WRN] Developers assume no liability and are not responsible for any misuse or damage.
[INF] Listing assets from AWS (prod) provider.
abc.com
example.com
1.1.1.1
2.2.2.2
3.3.3.3
4.4.4.4
5.5.5.5
6.6.6.6
[INF] Found 2 hosts and 6 IPs from AWS service (prod)
```
## Running cloudlist with Nuclei

Scanning assets from various cloud providers with nuclei for security assessments:- 

```bash
▶ cloudlist -silent | httpx -silent | nuclei -t cves/
```

# Supported providers

- AWS (Amazon web services)
  - EC2
  - Route53
- GCP (Google Cloud Platform)
  - Cloud DNS
- DO (DigitalOcean)
  - Instances
- SCW (Scaleway)
  - Instances

To see how to configure providers, check [PROVIDERS.md](https://github.com/projectdiscovery/cloudlist/blob/main/PROVIDERS.md).

# Todo

- [ ] Add support for Azure platform

# Cloudlist as a library

It's possible to use the library directly in your go programs. The following code snippets outline how to list assets from all or given cloud provider.

```go
package main

import (
	"context"
	"log"

	"github.com/projectdiscovery/cloudlist/pkg/inventory"
	"github.com/projectdiscovery/cloudlist/pkg/schema"
)

func main() {
	inventory, err := inventory.New(schema.Options{
		schema.OptionBlock{"provider": "digitalocean", "digitalocean_token": "ec405badb974fd3d891c9223245f9ab5871c127fce9e632c8dc421edd46d7242"},
	})
	if err != nil {
		log.Fatalf("%s\n", err)
	}

	for _, provider := range inventory.Providers {
		resources, err := provider.Resources(context.Background())
		if err != nil {
			log.Fatalf("%s\n", err)
		}
		for _, resource := range resources.Items {
			_ = resource // Do something with the resource
		}
	}
}
```

## Acknowledgments

Thank you for inspiration

* [Smogcloud](https://github.com/BishopFox/smogcloud)
* [Cloudmapper](https://github.com/duo-labs/cloudmapper)

## License

cloudlist is made with 🖤 by the [projectdiscovery](https://projectdiscovery.io) team and licensed under [MIT](https://github.com/projectdiscovery/proxify/blob/master/LICENSE.MD)
