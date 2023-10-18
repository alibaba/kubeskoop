export interface UserInfo {
  name: string;
  avatar: string;
  id: string;
}

export interface LoginParams {
  username?: string;
  password?: string;
  autoLogin?: boolean;
  phone?: string;
  code?: string;
}

export interface LoginResult {
  success?: boolean;
  userType?: 'user' | 'admin' | 'guest';
}
