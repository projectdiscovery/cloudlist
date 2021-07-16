# Providers

### Amazon Web Services (AWS)

Amazon Web Services can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: aws
  # profile is the name of the provider profile
  profile: staging
  # aws_access_key is the access key for AWS account
  aws_access_key: AKIAXXXXXXXXXXXXXX
  # aws_secret_key is the secret key for AWS account
  aws_secret_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

`aws_access_key` and `aws_secret_key` can be generated in the IAM console. We recommend creating a new IAM user with `Read Only` permissions and providing the access token for the user.

Scopes Required - 
1. EC2
2. Route53

References - 
1. https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_examples_iam_read-only-console.html
2. https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html
   
### Google Cloud Platform (GCP)

Google Cloud Platform can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: gcp
  # profile is the name of the provider profile
  profile: staging
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
 # profile is the name of the provider profile
 profile: staging
 # client_id is the client ID of registered application of the azure account (not requuired if using cli auth)
 client_id: xxxxxxxxxxxxxxxxxxxxxxxxx
 # client_secret is the secret ID of registered application of the zure account (not requuired if using cli auth)
 client_secret: xxxxxxxxxxxxxxxxxxxxx
 # tenant_id is the tenant ID of registered application of the azure account (not requuired if using cli auth)
 tenant_id: xxxxxxxxxxxxxxxxxxxxxxxxx
 #subscription_id is the azure subscription id
 subscription_id: xxxxxxxxxxxxxxxxxxx
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
  digitalocean_token: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
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
  scaleway_access_key: SCWXXXXXXXXXXXXXX
  # scaleway_access_token is the access token for scaleway API
  scaleway_access_token: xxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxx
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
  email: user@domain.com
  # api_key is the api_key for cloudflare
  api_key: xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx
```

`api_key` can be generated from Cloudflare API Key manager. It needs to be Global API Key due to limitation of cloudflare new API tokens.

References - 
1. https://developers.cloudflare.com/api/keys
   
### Heroku

Heroku can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: heroku
 # profile is the name of the provider profile
 profile: staging
 # heroku_api_token is the authorization token for Heroku account
 heroku_api_token: cf0e05d9-4eca-4948-a012-b9xxxxxxxxxx
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
 # profile is the name of the provider profile
 profile: staging
 # fastly_api_key is the personal API token for fastly account
 fastly_api_key: XX-XXXXXXXXXXXXXXXXXXXXXX-
```

`fastly_api_key` can be generated from https://manage.fastly.com/account/personal/tokens

References - 
1. https://docs.fastly.com/en/guides/using-api-tokens#creating-api-tokens


### Linode

Linode can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
 provider: linode
 # profile is the name of the provider profile
 profile: staging
 # linode_personal_access_token is the personal access token for Linode account
 linode_personal_access_token: XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX
```

`linode_personal_access_token` can be created from https://cloud.linode.com/profile/tokens. Minimum scope needed is `Read Only` for `Linodes` resource.

References - 
1. https://www.linode.com/docs/guides/getting-started-with-the-linode-api/#get-an-access-token


### Namecheap

Namecheap can be integrated by using the following configuration block.

```yaml
- # provider is the name of the provider
  provider: namecheap
  # profile is the name of the provider profile
  profile: staging
  # namecheap_api_key is the api key for namecheap account
  namecheap_api_key: xxxxxxxxxxxxxxxxxx
  # namecheap_user_name is the username of the namecheap account
  namecheap_user_name: XXXXXXXX
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
 # profile is the name of the provider profile
 profile: staging
 # alibaba_region_id is the region id of the resources
 alibaba_region_id: ap-XXXXXXX
 # alibaba_access_key is the access key ID for alibaba cloud account
 alibaba_access_key: XXXXXXXXXXXXXXXXXXXX
 # alibaba_access_key_secret is the secret access key for alibaba cloud account
 alibaba_access_key_secret: XXXXXXXXXXXXXXXX
```

Alibaba Cloud Access Key ID and Secret can be created by visiting https://ram.console.aliyun.com/manage/ak


References - 
- https://www.alibabacloud.com/help/faq-detail/142101.htm
- https://www.alibabacloud.com/help/doc-detail/53045.htm 
