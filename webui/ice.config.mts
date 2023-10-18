import { defineConfig } from '@ice/app';
import request from '@ice/plugin-request';
import store from '@ice/plugin-store';
import auth from '@ice/plugin-auth';

// The project config, see https://v3.ice.work/docs/guide/basic/config
const minify = process.env.NODE_ENV === 'production' ? 'swc' : false;
export default defineConfig(() => ({
  ssg: false,
  minify,
  plugins: [request(), store(), auth()],
  routes: {
    ignoreFiles: ['**/components/**'],
  },
  compileDependencies: false,
}));
