Pluuuummminnggg

1. DNS and CDN w/ cloudflare (set up terraform)
2. Blob storage with Vultr (set up terraform)
3. Most endpoints are just immutable, but the `/package/versions/*name` endpoint is not. Set up a cloudflare worker to read from the blob index and generate the json response we need. Maybe cache with a short ttl.

https://docs.github.com/en/actions/learn-github-actions/events-that-trigger-workflows#workflow_dispatch
https://docs.github.com/en/rest/reference/actions#create-a-workflow-dispatch-event

```
GITHUB_RUN_ATTEMPT=1
GITHUB_RUN_ID=1538965021
GITHUB_RUN_NUMBER=536
```

Job builders are run as github actions. I think we can run a cloudflare worker that just talks to the github api. Status is checked by checking the status of the job. We can return the job id to the client and use it to confirm the value of the github worker job.

Should be pretty zero-maintenance then.

Store terraform state in vultr
https://www.terraform.io/docs/language/settings/backends/s3.html

https://github.com/octokit/octokit.js

So the CF Worker gets a request for a job. Hits the github api to start an action runner. Sends the invocation ID back to the client. Can fetch logs when
