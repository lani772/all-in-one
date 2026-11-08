// CI-only stand-in for the AppDeploy runtime SDK. Never used in production builds.
export const api = {
  get: async () => ({ data: {} }),
  post: async () => ({ data: {} }),
  put: async () => ({ data: {} }),
  delete: async () => ({ data: {} }),
};
export const auth = {
  getUser: async () => null,
  signInWithRedirect: async () => undefined,
};
