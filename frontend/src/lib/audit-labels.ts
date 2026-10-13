import type { Locale } from './locale'

const actions: Record<string, [string, string]> = {
  'auth.login': ['Жүйеге кіру', 'Вход в систему'], 'auth.logout': ['Жүйеден шығу', 'Выход из системы'],
  'user.created': ['Қызметкерді қосу', 'Создание сотрудника'], 'user.updated': ['Қызметкерді өзгерту', 'Изменение сотрудника'],
  'department.created': ['Бөлімді қосу', 'Создание отдела'], 'department.updated': ['Бөлімді өзгерту', 'Изменение отдела'],
  'category.created': ['Санатты қосу', 'Создание категории'], 'category.updated': ['Санатты өзгерту', 'Изменение категории'],
  'article.created': ['Нұсқаулықты қосу', 'Создание инструкции'], 'article.updated': ['Нұсқаулықты өзгерту', 'Изменение инструкции'], 'article.deleted': ['Нұсқаулықты жою', 'Удаление инструкции'],
  'media.uploaded': ['Файлды жүктеу', 'Загрузка файла'], 'ticket.created': ['Өтінімді жіберу', 'Создание заявки'], 'ticket.updated': ['Өтінімді өзгерту', 'Изменение заявки'],
  'ticket.status_changed': ['Өтінім күйін өзгерту', 'Изменение статуса заявки'], 'ticket.commented': ['Өтінімге хабарлама', 'Сообщение к заявке'],
  'ticket.attachments_added': ['Скриншоттарды тіркеу', 'Добавление скриншотов'], 'ticket.converted': ['Өтінімнен нұсқаулық жасау', 'Создание инструкции из заявки'],
  'settings.updated': ['Баптауларды өзгерту', 'Изменение настроек'],
}
const entities: Record<string, [string, string]> = { user: ['Қызметкер', 'Сотрудник'], department: ['Бөлім', 'Отдел'], category: ['Санат', 'Категория'], article: ['Нұсқаулық', 'Инструкция'], ticket: ['Өтінім', 'Заявка'], media: ['Файл', 'Файл'], session: ['Сеанс', 'Сеанс'], application_settings: ['Қолданба баптаулары', 'Настройки приложения'] }
export function auditActionLabel(value: string, locale: Locale) { return actions[value]?.[locale === 'kk' ? 0 : 1] ?? value }
export function auditEntityLabel(value: string, locale: Locale) { const aliases: Record<string, string> = { knowledge_article: 'article', knowledge_category: 'category' }; return entities[aliases[value] ?? value]?.[locale === 'kk' ? 0 : 1] ?? value }
export function auditEntityLink(type: string, id: string | null) { return !id ? null : type === 'ticket' ? `/tickets/${encodeURIComponent(id)}` : type === 'article' || type === 'knowledge_article' ? `/knowledge/articles/${encodeURIComponent(id)}` : null }
