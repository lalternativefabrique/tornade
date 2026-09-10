type AdminScreenProps = {
  children: React.ReactNode
}

export function AdminScreen({ children }: AdminScreenProps) {
  return (
    <div className="lalt-admin flex min-h-screen items-center justify-center bg-background px-6 py-14">
      {children}
    </div>
  )
}
