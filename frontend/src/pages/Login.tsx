import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Eye, EyeOff, LockKeyhole, ShieldCheck } from 'lucide-react'
import { Navigate } from 'react-router-dom'
import { Button } from '../components/ui/button'
import { FormField, fieldDescription } from '../components/forms/FormField'
import { useLocale } from '../lib/locale'
import { useAuth } from '../lib/auth'
import { useErrorMessage } from '../lib/notifications'

export function Login() {
  const { t } = useLocale()
  const auth = useAuth()
  const errorMessage = useErrorMessage()
  const [visible, setVisible] = useState(false)
  const [error, setError] = useState<unknown>(null)
  const schema = z.object({ email: z.string().trim().email(t.emailInvalid).max(254, t.emailInvalid), password: z.string().min(1, t.passwordRequired) })
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { email: '', password: '' } })
  if (auth.status === 'loading') return <div className="session-state" role="status" aria-busy="true">{t.sessionLoading}</div>
  if (auth.user) return <Navigate to={auth.user.account_scope === 'SCHOOL' ? '/school' : '/'} replace />
  const submit = form.handleSubmit(async values => {
    setError(null)
    try { await auth.login(values.email, values.password) }
    catch (cause) { setError(cause) }
  })
  return <div className="login-page">
    <div className="login-intro"><div className="eyebrow"><span />{t.internalPortal}</div><h1>{t.accessTitle}</h1><p>{t.signInDescription}</p></div>
    <section className="login-card" aria-labelledby="login-title"><span className="login-lock"><LockKeyhole size={28} strokeWidth={1.5} aria-hidden="true" /></span><h2 id="login-title">{t.signInTitle}</h2>
      {auth.status === 'error' && <p className="inline-error" role="alert">{t.sessionUnavailable}</p>}
      <form onSubmit={submit} noValidate className="auth-form">
        <FormField id="login-email" label={t.email} error={form.formState.errors.email?.message}><input id="login-email" type="email" autoComplete="username" maxLength={254} {...form.register('email')} aria-invalid={!!form.formState.errors.email} aria-describedby={fieldDescription('login-email', form.formState.errors.email?.message)} disabled={form.formState.isSubmitting} /></FormField>
        <FormField id="login-password" label={t.password} error={form.formState.errors.password?.message}><div className="password-input"><input id="login-password" type={visible ? 'text' : 'password'} autoComplete="current-password" {...form.register('password')} aria-invalid={!!form.formState.errors.password} aria-describedby={fieldDescription('login-password', form.formState.errors.password?.message)} disabled={form.formState.isSubmitting} /><button type="button" onClick={() => setVisible(!visible)} aria-label={visible ? t.passwordHide : t.passwordVisibility}>{visible ? <EyeOff size={18} aria-hidden="true" /> : <Eye size={18} aria-hidden="true" />}</button></div></FormField>
        {error !== null && <p className="inline-error" role="alert">{errorMessage(error)}</p>}
        <Button className="login-submit" type="submit" disabled={form.formState.isSubmitting}>{form.formState.isSubmitting ? t.signingIn : t.signInSubmit}</Button>
      </form>
      <div className="login-security"><ShieldCheck size={18} aria-hidden="true" /><div><strong>{t.protectedWorkspace}</strong><span>{t.authRoles}</span></div></div>
    </section>
  </div>
}
