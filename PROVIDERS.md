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
   
   