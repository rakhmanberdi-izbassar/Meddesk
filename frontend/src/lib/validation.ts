import { z } from 'zod'

export function passwordIsValid(password: string) {
  const bytes = new TextEncoder().encode(password).length
  return bytes >= 12 && bytes <= 72
}
export function passwordSchema(message: string) {
  return z.string().refine(passwordIsValid, message)
}
export function nameSchema(message: string) {
  return z.string().trim().refine(value => Array.from(value).length >= 1 && Array.from(value).length <= 150 && !/[\p{Cc}\p{Cf}]/u.test(value), message)
}
