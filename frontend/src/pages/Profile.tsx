import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { UserRound, ShieldCheck } from 'lucide-react'
import { Button } from '../components/ui/button'
import { FormField, fieldDescription } from '../components/forms/FormField'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage, useToast } from '../lib/notifications'
import { userResultSchema } from '../lib/api'
import { nameSchema } from '../lib/validation'

export function Profile() {
  const { t } = useLocale()
  const auth = useAuth()
  const notify = useToast()
  const errorMessage = useErrorMessage()
  const schema = z.object({ display_name: nameSchema(t.nameRule) })
  const form = useForm<z.infer<typeof schema>>({ resolver: zodResolver(schema), defaultValues: { display_name: auth.user?.display_name ?? '' } })
  const user = auth.user!
  const { reset, formState: { isDirty } } = form
  useEffect(() => { if (!isDirty) reset({ display_name: user.display_name }) }, [isDirty, reset, user.display_name])
  const role = user.role === 'ADMIN' ? t.adminRole : user.role === 'IT_ENGINEER' ? t.engineerRole : t.employeeRole
  const submit = form.handleSubmit(async values => {
    try {
      const result = await auth.api('/profile', userResultSchema, { method: 'PATCH', body: values })
      auth.updateUser(result.user)
      form.reset({ display_name: result.user.display_name })
      notify({ kind: 'success', message: t.profileSaved })
    } catch (error) { notify({ kind: 'error', message: errorMessage(error) }) }
  })
  return <><div className="page-heading"><div><div className="eyebrow"><span />{t.account}</div><h1>{t.profile}</h1><p>{t.profileDescription}</p></div></div>
    <div className="profile-grid"><section className="content-card"><div className="card-heading"><UserRound size={22} aria-hidden="true" /><h2>{user.display_name}</h2></div><form onSubmit={submit} noValidate className="account-form"><FormField id="profile-name" label={t.displayName} error={form.formState.errors.display_name?.message}><input id="profile-name" {...form.register('display_name')} autoComplete="name" aria-invalid={!!form.formState.errors.display_name} aria-describedby={fieldDescription('profile-name', form.formState.errors.display_name?.message)} disabled={form.formState.isSubmitting} /></FormField><Button type="submit" disabled={form.formState.isSubmitting || !form.formState.isDirty}>{form.formState.isSubmitting ? t.saving : t.save}</Button></form></section>
      <section className="content-card"><div className="card-heading"><ShieldCheck size={22} aria-hidden="true" /><h2>{t.account}</h2></div><dl className="account-details"><div><dt>{t.email}</dt><dd>{user.email}</dd></div><div><dt>{t.department}</dt><dd>{user.department_name}</dd></div><div><dt>{t.role}</dt><dd>{role}</dd></div></dl><p className="card-note">{t.profileManaged}</p><p className="card-note">{t.accountSecurity}</p></section></div>
  </>
}
