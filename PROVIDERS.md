# Providers

### Amazon Web Services (AWS)

Amazon Web Services can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: aws
  # id is the name defined by user for filtering (optional)
  id: staging
  # aws_access_key is the access key for AWS account
  aws_access_key: $AWS_ACCESS_KEY
  # aws_secret_key is the secret key for AWS account
  aws_secret_key: $AWS_SECRET_KEY
  # aws_session_token session token for temporary security credentials retrieved via STS (optional)
  aws_session_token: $AWS_SESSION_TOKEN
```

`aws_access_key` and `aws_secret_key` can be generated in the IAM console. We recommend creating a new IAM user with `Read Only` permissions and providing the access token for the user.

Scopes Required - 
1. EC2
2. Route53

References - 
1. https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_examples_iam_read-only-console.html
2. https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html
3. https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_temp_request.html
   
### Google Cloud Platform (GCP)

Google Cloud Platform can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: gcp
  # id is the name defined by user for filtering (optional)
  id: staging
  # gcp_service_account_key is the key token of service account.
  gcp_service_account_key: '{}'
```

`gcp_service_account_key` can be retrieved by creating a new service account. To do so, create service account with Read Only access to `cloudresourcemanager` and `dns` scopes in IAM. Next, generate a new account key for the Service Account by following steps in Reference 2. This should give you a json which can be pasted in a single line in the `gcp_service_account_key`.

Scopes Required - 
1. Cloud DNS

References - 
1. https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_examples_iam_read-only-console.html
2. https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html


### Microsoft Azure

Microsoft Azure can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: azure
 # id is the name defined by user for filtering (optional)
 id: staging
 # client_id is the client ID of registered application of the azure account (not requuired if using cli auth)
 client_id: $AZURE_CLIENT_ID
 # client_secret is the secret ID of registered application of the zure account (not requuired if using cli auth)
 client_secret: $AZURE_CLIENT_SECRET
 # tenant_id is the tenant ID of registered application of the azure account (not requuired if using cli auth)
 tenant_id: $AZURE_TENANT_ID
 #subscription_id is the azure subscription id
 subscription_id: $AZURE_SUBSCRIPTION_ID
 #use_cli_auth if set to true cloudlist will use azure cli auth
 use_cli_auth: true
```

`tenant_id`, `client_id`, `client_secret` can be obtained/generated from   `All services` > `Azure Active Directory` > `App registrations`
`subscription_id` can be retrieved from  `All services` > `Subscriptions`

To use cli auth set `use_cli_auth` value to `true` and run `az login` in the terminal


References - 
1. https://docs.microsoft.com/en-us/cli/azure/create-an-azure-service-principal-azure-cli
2. https://docs.microsoft.com/en-us/cli/azure/ad/sp?view=azure-cli-latest#az_ad_sp_create_for_rbac
3. https://docs.microsoft.com/en-us/cli/azure/authenticate-azure-cli





   

### DigitalOcean (DO)

Digitalocean can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: do
  # digitalocean_token is the API key for digitalocean cloud platform
  digitalocean_token: $DIGITALOCEAN_TOKEN
```

`digitalocean_token` can be generated from the Digitalocean Control Panel. We recommend only giving Read Access to the token.

References - 
1. https://www.digitalocean.com/docs/apis-clis/api/create-personal-access-token/
   
### Scaleway (SCW)

Scaleway can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: scw
  # scaleway_access_key is the access key for scaleway API
  scaleway_access_key: $SCALEWAY_ACCESS_KEY
  # scaleway_access_token is the access token for scaleway API
  scaleway_access_token: $SCALEWAY_ACCESS_TOKEN
```

`scaleway_access_key` and `scaleway_access_token` can be generated from the Credentials Options in scaleway console.

References - 
1. https://www.scaleway.com/en/docs/generate-api-keys/
   
### Cloudflare 

Cloudflare can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: cloudflare
  # email is the email for cloudflare
  email: $CF_EMAIL
  # api_key is the api_key for cloudflare
  api_key: $CF_API_KEY
  # api_token is the scoped_api_token for cloudflare (optional)
  api_token: $CF_API_TOKEN
```

`api_key` can be generated from Cloudflare API Key manager. It needs to be Global API Key due to limitation of cloudflare new API tokens.

References - 
1. https://developers.cloudflare.com/api/keys
   
### Heroku

Heroku can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: heroku
 # id is the name defined by user for filtering (optional)
 id: staging
 # heroku_api_token is the authorization token for Heroku account
 heroku_api_token: $HEROKU_API_TOKEN
```

`heroku_api_token` can be generated from https://dashboard.heroku.com/account/applications/authorizations/new

It can also be created with the Heroku CLI by running:
```bash
$ heroku authorizations:create -d "brief description of token"
Creating OAuth Authorization... done
Client:      <none>
ID:          a6e98151-f242-4592-b107-25fbac5ab410
Description: brief description of token
Scope:       global
Token:       cf0e05d9-4eca-4948-a012-b9xxxxxxxxxx
Updated at:  Fri Jun 16 2021 13:26:56 GMT-0700 (PDT) (less than a minute ago)
```


References - 
1. https://devcenter.heroku.com/articles/platform-api-quickstart#authentication

### Fastly

Fastly can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: fastly
 # id is the name defined by user for filtering (optional)
 id: staging
 # fastly_api_key is the personal API token for fastly account
 fastly_api_key: $FASTLY_API_KEY
```

`fastly_api_key` can be generated from https://manage.fastly.com/account/personal/tokens

References - 
1. https://docs.fastly.com/en/guides/using-api-tokens#creating-api-tokens


### Linode

Linode can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: linode
 # id is the name defined by user for filtering (optional)
 id: staging
 # linode_personal_access_token is the personal access token for Linode account
 linode_personal_access_token: $LINODE_PERSONAL_ACCESS_TOKEN
```

`linode_personal_access_token` can be created from https://cloud.linode.com/id/tokens. Minimum scope needed is `Read Only` for `Linodes` resource.

References - 
1. https://www.linode.com/docs/guides/getting-started-with-the-linode-api/#get-an-access-token


### Namecheap

Namecheap can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: namecheap
  # id is the name defined by user for filtering (optional)
  id: staging
  # namecheap_api_key is the api key for namecheap account
  namecheap_api_key: $NAMECHEAP_API_KEY
  # namecheap_user_name is the username of the namecheap account
  namecheap_user_name: $NAMECHEAP_USER_NAME
```

Namecheap API Access can be enabled by visiting https://ap.www.namecheap.com/settings/tools/apiaccess/ and then:
- Toggle ON API Access switch
- Add your public IP to Whitelistted IPs 


References - 
- https://www.namecheap.com/support/api/intro/
    - Enabling API Access
    - Whitelisting IP


### Alibaba Cloud

Alibaba Cloud can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: alibaba
 # id is the name defined by user for filtering (optional)
 id: staging
 # alibaba_region_id is the region id of the resources
 alibaba_region_id: $ALIBABA_REGION_ID
 # alibaba_access_key is the access key ID for alibaba cloud account
 alibaba_access_key: $ALIBABA_ACCESS_KEY
 # alibaba_access_key_secret is the secret access key for alibaba cloud account
 alibaba_access_key_secret: $ALIBABA_ACCESS_KEY_SECRET
```

Alibaba Cloud Access Key ID and Secret can be created by visiting https://ram.console.aliyun.com/manage/ak


References - 
- https://www.alibabacloud.com/help/faq-detail/142101.htm
- https://www.alibabacloud.com/help/doc-detail/53045.htm 


### Terraform

Terraform can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: terraform
 # id is the name defined by user for filtering (optional)
 id: staging
 #tf_state_file is the location of terraform state file (terraform.tfsate) 
 tf_state_file: path/to/terraform.tfstate
```
### Hashicorp Consul

Hashicorp Consul can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: consul
  # consul_url is the url for consul server
  consul_url: http://localhost:8500/
  # id is the name defined by user for filtering (optional)
  id: staging
  # consul_ca_file is the path to consul CA file
  # consul_ca_file: <path-to-ca-file>.pem
  # consul_cert_file is the path to consul Certificate file
  # consul_cert_file: <path-to-cert-file>.pem
  # consul_key_file is the path to consul Certificate Key file
  # consul_key_file: <path-to-key-file>.pem
  # consul_http_token is the consul authentication token
  # consul_http_token: <consul-token>
  # consul_http_auth is the consul http auth value
  # consul_http_auth: <consul-http-auth-value>
```

Specifying https in the `consul_url` automatically turns SSL to on. All the fields are optional except the `consul_url`.

References - 
- https://www.consul.io/api-docs

### Hashicorp Nomad

Hashicorp Nomad can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: nomad
  # nomad_url is the url for nomad server
  nomad_url: http://127.0.0.1:4646/
  # id is the name defined by user for filtering (optional)
  id: staging
  # nomad_ca_file is the path to nomad CA file
  # nomad_ca_file: <path-to-ca-file>.pem
  # nomad_cert_file is the path to nomad Certificate file
  # nomad_cert_file: <path-to-cert-file>.pem
  # nomad_key_file is the path to nomad Certificate Key file
  # nomad_key_file: <path-to-key-file>.pem
  # nomad_token is the nomad authentication token
  # nomad_token: <nomad-token>
  # nomad_http_auth is the nomad http auth value
  # nomad_http_auth: <nomad-http-auth-value>

```

Specifying https in the `nomad_url` automatically turns SSL to on. All the fields are optional except the `nomad_url`.

References - 
- https://www.nomadproject.io/api-docs

### Hetzner Cloud

Hetzner Cloud can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: hetzner
 # id is the name defined by user for filtering (optional)
 id: staging
 # auth_token is the is the hetzner authentication token
 auth_token: $HETZNER_AUTH_TOKEN
```

References -
- https://docs.hetzner.cloud/#authentication

### Kubernetes

Kubernetes can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: kubernetes
 # id is the name defined by user for filtering (optional)
 id: staging
 # kubeconfig is the path to kubeconfig file
 kubeconfig: path/to/kubeconfig
 # context is the context to be used from kubeconfig file (optional), if not specified, current-context will be used
 context: <context-name>
```

References -
- https://www.redhat.com/sysadmin/kubeconfig
- https://docs.aws.amazon.com/eks/latest/userguide/create-kubeconfig.html
- https://learn.microsoft.com/en-us/azure/aks/control-kubeconfig-access#get-and-verify-the-configuration-information
- https://cloud.google.com/kubernetes-engine/docs/how-to/cluster-access-for-kubectl#store_info
