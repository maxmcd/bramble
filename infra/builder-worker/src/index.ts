export { BuilderWorker } from "./builder-worker";

export default {
  async fetch(request: Request, env: Env) {
    // https://developers.cloudflare.com/workers/runtime-apis/durable-objects#accessing-a-durable-object-from-a-worker
    let id = env.BRAMBLE_DO.idFromName("bramble");
    let obj = env.BRAMBLE_DO.get(id);
    return await obj.fetch(request);
  },
};

interface Env {
  BRAMBLE_DO: DurableObjectNamespace;
  GITHUB_TOKEN: string;
}
