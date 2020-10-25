
- need to be able to specify multiple versions per repo
- will probably want to do that through renaming
- could use semver, but will need to use semver+hash
- could allow one to specify by branch name or hash, will convert to hash
- upgrading will take the latest hash on the default branch
- if you use semver we can look for the latest version tagged with semver
