import type { LoginParams, LoginResult } from '@/interfaces/user';
import { request } from '@ice/plugin-request/request';
import Cookies from 'js-cookie';

export async function login(data: LoginParams): Promise<LoginResult> {
  return await request.post('/auth/login', data, { withCredentials: true });
}

export async function fetchUserInfo() {
  return await request.get('/auth/info');
}

export async function logout() {
  Cookies.remove('jwt');
}
