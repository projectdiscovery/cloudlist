<h1 align="center">
  <img src="static/cloudlist-logo.png" alt="cloudlist" width="400px"></a>
  <br>
</h1>


<p align="center">
<a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/license-MIT-_red.svg"></a>
<a href="https://github.com/projectdiscovery/cloudlist/issues"><img src="https://img.shields.io/badge/contributions-welcome-brightgreen.svg?style=flat"></a>
<a href="https://goreportcard.com/badge/github.com/projectdiscovery/cloudlist"><img src="https://goreportcard.com/badge/github.com/projectdiscovery/cloudlist"></a>
<a href="https://github.com/projectdiscovery/cloudlist/releases"><img src="https://img.shields.io/github/release/projectdiscovery/cloudlist"></a>
<a href="https://twitter.com/pdiscoveryio"><img src="https://img.shields.io/twitter/follow/pdiscoveryio.svg?logo=twitter"></a>
<a href="https://discord.gg/projectdiscovery"><img src="https://img.shields.io/discord/695645237418131507.svg?logo=discord"></a>
</p>

<p align="center">
  <a href="#features">Features</a> •
  <a href="https://docs.projectdiscovery.io/tools/cloudlist/install">Installation</a> •
  <a href="https://docs.projectdiscovery.io/tools/cloudlist/usage">Usage</a> •
  <a href="https://docs.projectdiscovery.io/tools/cloudlist/running#configuration-file">Configuration</a> •
  <a href="https://docs.projectdiscovery.io/tools/cloudlist/running">Running cloudlist</a> •
  <a href="https://docs.projectdiscovery.io/tools/cloudlist/running#supported-providers">Supported providers</a> •
  <a href="https://docs.projectdiscovery.io/tools/cloudlist/running#cloudlist-as-a-library">Library</a> •
  <a href="https://discord.gg/projectdiscovery">Join Discord</a>
</p>


Cloudlist is a multi-cloud tool for getting Assets from Cloud Providers. This is intended to be used by the blue team to augment Attack Surface Management efforts by maintaining a centralized list of assets across multiple clouds with very little configuration efforts.


# Features

<h1 align="left">
  <img src="static/cloudlist-run.png" alt="cloudlist" width="700px"></a>
  <br>
</h1>


 - List Cloud assets with multiple configurations
 - Multiple Cloud providers support
 - Multiple output format support
 - Multiple filters support
 - Highly extensible making adding new providers a breeze
 - **stdout** support to work with other tools in pipelines

# Usage

```sh
cloudlist -h
```

This will display help for the tool. Here are all the switches it supports.

```yaml
Usage:
  ./cloudlist [flags]

Flags:
CONFIGURATION:
   -config string                cloudlist flag config file (default "$HOME/.config/cloudlist/config.yaml")
   -pc, -provider-config string  provider config file (default "$HOME/.config/cloudlist/provider-config.yaml")

FILTERS:
   -p, -provider string[]  display results for given providers (comma-separated)
   -id string[]            display results for given ids (comma-separated)
   -host                   display only hostnames in results
   -ip                     display only ips in results
   -ep, -exclude-private   exclude private ips in cli output

OUTPUT:
   -o, -output string  output file to write results
   -json               write output in json format
   -version            display version of cloudlist
   -v                  display verbose output
   -silent             display only results in output
```

# Contribution

Please check [PROVIDERS.md](https://github.com/projectdiscovery/cloudlist/blob/main/PROVIDERS.md) and [DESIGN.md](https://github.com/projectdiscovery/cloudlist/blob/main/DESIGN.md) to include support for new cloud providers in Cloudlist.


- Fork this project
- Create your feature branch (`git checkout -b new-provider`)
- Commit your changes (`git commit -am 'Added new cloud provider'`)
- Push to the branch (`git push origin new-provider`)
- Create new Pull Request

## Acknowledgments

Thank you for inspiration

* [Smogcloud](https://github.com/BishopFox/smogcloud)
* [Cloudmapper](https://github.com/duo-labs/cloudmapper)

## License

cloudlist is made with 🖤 by the [projectdiscovery](https://projectdiscovery.io) team and licensed under [MIT](https://github.com/projectdiscovery/cloudlist/blob/main/LICENSE.md)
