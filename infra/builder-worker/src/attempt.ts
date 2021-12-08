import { Octokit } from "octokit";

const octokit = new Octokit({ auth: "" });

(async function() {
  let resp = await octokit.request("GET /repos/{owner}/{repo}/actions/jobs/{job_id}/logs", {
    owner: "maxmcd",
    repo: "bramble",
    job_id: 4452262973,
  });
  console.log(resp);
})();
