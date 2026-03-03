package vercel

type ListProjectsRequest struct {
	// Limit the number of projects returned.
	// Required: No
	Limit int64 `json:"limit,omitempty"`

	// The updatedAt point64where the list should start.
	// Required: No
	Since int64 `json:"since,omitempty"`

	// The updatedAt point64where the list should end.
	// Required: No
	Until int64 `json:"until,omitempty"`

	// Search projects by the name field.
	// Required: No
	Search string `json:"string,omitempty"`
}

type ListProjectsResponse struct {
	Projects []Project `json:"projects"`
}

// Project houses all the information vercel offers about a project via their api
type Project struct {
	Accountid string `json:"accountid"`
	Alias     []struct {
		ConfiguredBy        string `json:"configuredBy"`
		ConfiguredChangedAt int64  `json:"configuredChangedAt"`
		CreatedAt           int64  `json:"createdAt"`
		Deployment          struct {
			Alias         []string      `json:"alias"`
			AliasAssigned int64         `json:"aliasAssigned"`
			Builds        []interface{} `json:"builds"`
			CreatedAt     int64         `json:"createdAt"`
			CreatedIn     string        `json:"createdIn"`
			Creator       struct {
				Uid         string `json:"uid"`
				Email       string `json:"email"`
				Username    string `json:"username"`
				GithubLogin string `json:"githubLogin"`
			} `json:"creator"`
			DeploymentHostname string `json:"deploymentHostname"`
			Forced             bool   `json:"forced"`
			Id                 string `json:"id"`
			Meta               struct {
				GithubCommitRef         string `json:"githubCommitRef"`
				GithubRepo              string `json:"githubRepo"`
				GithubOrg               string `json:"githubOrg"`
				GithubCommitSha         string `json:"githubCommitSha"`
				GithubRepoid            string `json:"githubRepoid"`
				GithubCommitMessage     string `json:"githubCommitMessage"`
				GithubCommitAuthorLogin string `json:"githubCommitAuthorLogin"`
				GithubDeployment        string `json:"githubDeployment"`
				GithubCommitOrg         string `json:"githubCommitOrg"`
				GithubCommitAuthorName  string `json:"githubCommitAuthorName"`
				GithubCommitRepo        string `json:"githubCommitRepo"`
				GithubCommitRepoid      string `json:"githubCommitRepoid"`
			} `json:"meta"`
			Name       string `json:"name"`
			Plan       string `json:"plan"`
			Private    bool   `json:"private"`
			ReadyState string `json:"readyState"`
			Target     string `json:"target"`
			Teamid     string `json:"teamid"`
			Type       string `json:"type"`
			URL        string `json:"url"`
			Userid     string `json:"userid"`
			WithCache  bool   `json:"withCache"`
		} `json:"deployment"`
		Domain      string `json:"domain"`
		Environment string `json:"environment"`
		Target      string `json:"target"`
	} `json:"alias"`
	Analytics struct {
		Id         string `json:"id"`
		EnabledAt  int64  `json:"enabledAt"`
		DisabledAt int64  `json:"disabledAt"`
		CanceledAt int64  `json:"canceledAt"`
	} `json:"analytics"`
	AutoExposeSystemEnvs bool   `json:"autoExposeSystemEnvs"`
	BuildCommand         string `json:"buildCommand"`
	CreatedAt            int64  `json:"createdAt"`
	DevCommand           string `json:"devCommand"`
	DirectoryListing     bool   `json:"directoryListing"`
	Env                  []struct {
		Type            string      `json:"type"`
		Id              string      `json:"id"`
		Key             string      `json:"key"`
		Value           string      `json:"value"`
		Target          []string    `json:"target"`
		Configurationid interface{} `json:"configurationid"`
		UpdatedAt       int64       `json:"updatedAt"`
		CreatedAt       int64       `json:"createdAt"`
	} `json:"env"`
	Framework                       string `json:"framework"`
	Id                              string `json:"id"`
	InstallCommand                  string `json:"installCommand"`
	Name                            string `json:"name"`
	NodeVersion                     string `json:"nodeVersion"`
	OutputDirectory                 string `json:"outputDirectory"`
	PublicSource                    bool   `json:"publicSource"`
	RootDirectory                   string `json:"rootDirectory"`
	ServerlessFunctionRegion        string `json:"serverlessFunctionRegion"`
	SourceFilesOutsideRootDirectory bool   `json:"sourceFilesOutsideRootDirectory"`
	UpdatedAt                       int64  `json:"updatedAt"`
	Link                            struct {
		Type             string        `json:"type"`
		Repo             string        `json:"repo"`
		Repoid           int64         `json:"repoid"`
		Org              string        `json:"org"`
		GitCredentialid  string        `json:"gitCredentialid"`
		CreatedAt        int64         `json:"createdAt"`
		UpdatedAt        int64         `json:"updatedAt"`
		Sourceless       bool          `json:"sourceless"`
		ProductionBranch string        `json:"productionBranch"`
		DeployHooks      []interface{} `json:"deployHooks"`
		ProjectName      string        `json:"projectName"`
		ProjectNamespace string        `json:"projectNamespace"`
		Owner            string        `json:"owner"`
		Slug             string        `json:"slug"`
	} `json:"link"`
	LatestDeployments []struct {
		Alias         []string      `json:"alias"`
		AliasAssigned int64         `json:"aliasAssigned"`
		Builds        []interface{} `json:"builds"`
		CreatedAt     int64         `json:"createdAt"`
		CreatedIn     string        `json:"createdIn"`
		Creator       struct {
			Uid         string `json:"uid"`
			Email       string `json:"email"`
			Username    string `json:"username"`
			GithubLogin string `json:"githubLogin"`
		} `json:"creator"`
		DeploymentHostname string `json:"deploymentHostname"`
		Forced             bool   `json:"forced"`
		Id                 string `json:"id"`
		Meta               struct {
			GithubCommitRef         string `json:"githubCommitRef"`
			GithubRepo              string `json:"githubRepo"`
			GithubOrg               string `json:"githubOrg"`
			GithubCommitSha         string `json:"githubCommitSha"`
			GithubCommitAuthorLogin string `json:"githubCommitAuthorLogin"`
			GithubCommitMessage     string `json:"githubCommitMessage"`
			GithubRepoid            string `json:"githubRepoid"`
			GithubDeployment        string `json:"githubDeployment"`
			GithubCommitOrg         string `json:"githubCommitOrg"`
			GithubCommitAuthorName  string `json:"githubCommitAuthorName"`
			GithubCommitRepo        string `json:"githubCommitRepo"`
			GithubCommitRepoid      string `json:"githubCommitRepoid"`
		} `json:"meta"`
		Name       string      `json:"name"`
		Plan       string      `json:"plan"`
		Private    bool        `json:"private"`
		ReadyState string      `json:"readyState"`
		Target     interface{} `json:"target"`
		Teamid     string      `json:"teamid"`
		Type       string      `json:"type"`
		URL        string      `json:"url"`
		Userid     string      `json:"userid"`
		WithCache  bool        `json:"withCache"`
	} `json:"latestDeployments"`
	Targets struct {
		Production struct {
			Alias         []string      `json:"alias"`
			AliasAssigned int64         `json:"aliasAssigned"`
			Builds        []interface{} `json:"builds"`
			CreatedAt     int64         `json:"createdAt"`
			CreatedIn     string        `json:"createdIn"`
			Creator       struct {
				Uid         string `json:"uid"`
				Email       string `json:"email"`
				Username    string `json:"username"`
				GithubLogin string `json:"githubLogin"`
			} `json:"creator"`
			DeploymentHostname string `json:"deploymentHostname"`
			Forced             bool   `json:"forced"`
			Id                 string `json:"id"`
			Meta               struct {
				GithubCommitRef         string `json:"githubCommitRef"`
				GithubRepo              string `json:"githubRepo"`
				GithubOrg               string `json:"githubOrg"`
				GithubCommitSha         string `json:"githubCommitSha"`
				GithubRepoid            string `json:"githubRepoid"`
				GithubCommitMessage     string `json:"githubCommitMessage"`
				GithubCommitAuthorLogin string `json:"githubCommitAuthorLogin"`
				GithubDeployment        string `json:"githubDeployment"`
				GithubCommitOrg         string `json:"githubCommitOrg"`
				GithubCommitAuthorName  string `json:"githubCommitAuthorName"`
				GithubCommitRepo        string `json:"githubCommitRepo"`
				GithubCommitRepoid      string `json:"githubCommitRepoid"`
			} `json:"meta"`
			Name       string `json:"name"`
			Plan       string `json:"plan"`
			Private    bool   `json:"private"`
			ReadyState string `json:"readyState"`
			Target     string `json:"target"`
			Teamid     string `json:"teamid"`
			Type       string `json:"type"`
			URL        string `json:"url"`
			Userid     string `json:"userid"`
			WithCache  bool   `json:"withCache"`
		} `json:"production"`
	} `json:"targets"`
}
