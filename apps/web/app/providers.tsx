'use client'

import { GoogleOAuthProvider } from '@react-oauth/google'

// Google OAuth Client ID
// TODO: Move to environment variable
const GOOGLE_CLIENT_ID = '749099552710-jgkkffvfca1j68jc23b9tq962494m8c2.apps.googleusercontent.com'

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <GoogleOAuthProvider clientId={GOOGLE_CLIENT_ID}>
      {children}
    </GoogleOAuthProvider>
  )
}
