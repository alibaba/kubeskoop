export interface UserInfo {
  name: string;
  role: string;
}

export interface LoginParams {
  username?: string;
  password?: string;
}

export interface LoginResult {
  exp: number;
  orig_iat: number;
  role: string;
  user: string;
}
