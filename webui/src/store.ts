import { createStore } from 'ice';
import user from '@/models/user';
import dashboard from '@/models/dashboard'

export default createStore({
  user,
  dashboard
});
