export class AuthService {
  login(user: string): boolean {
    return user.length > 0;
  }
}

export function useSession() {
  return new AuthService();
}
