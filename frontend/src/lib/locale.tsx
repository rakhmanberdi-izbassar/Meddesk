import { phase2kk, phase2ru } from './phase2-locale'
import { phase3kk, phase3ru } from './phase3-locale'
import { phase4kk, phase4ru } from './phase4-locale'
import { phase5kk, phase5ru } from './phase5-locale'
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react'

export const localeStorageKey = 'meddesk.locale'
export type Locale = 'kk' | 'ru'

const kk = {
  clinic: 'Клиниканың IT қолдауы',
  workspace: 'Жұмыс кеңістігі',
  overview: 'Шолу',
  login: 'Жүйеге кіру',
  soon: 'ЖАҚЫНДА',
  knowledge: 'Білім базасы',
  tickets: 'Қолдау өтінімдері',
  analytics: 'Аналитика',
  notSignedIn: 'Жүйеге кірмегенсіз',
  internalPortal: 'Ішкі портал',
  mainNavigation: 'Негізгі навигация',
  openMenu: 'Мәзірді ашу',
  closeMenu: 'Мәзірді жабу',
  chooseLanguage: 'Интерфейс тілі',
  skip: 'Негізгі мазмұнға өту',
  foundation: '5-КЕЗЕҢ · АНАЛИТИКА ЖӘНЕ БАСҚАРУ',
  greeting: 'IT қолдауы, бір жерде.',
  greetingDescription: 'MedDesk — күнделікті техникалық мәселелерді шешуге арналған клиниканың бірыңғай жұмыс кеңістігі.',
  foundationNotice: 'Сіздің командаңызға арналған кеңістік',
  foundationDescription: 'Мәселені білім базасынан іздеп, қадамдық шешімді орындаңыз. Нұсқаулық көмектескенін белгілеңіз. Мәселе шешілмесе, IT маманына өтінім жіберіңіз.',
  currentPhase: 'Ағымдағы кезең',
  phaseOne: '05',
  phaseOneTitle: 'Аналитика және басқару',
  phaseOneDescription: 'Нақты көрсеткіштер, әрекеттер журналы және клиника баптаулары.',
  development: 'Әзірлеу нұсқасы',
  connectedTitle: 'Жүйе күйі',
  connectedDescription: 'API мен дерекқордың ағымдағы қолжетімділігі.',
  refresh: 'Жаңарту',
  refreshing: 'Тексерілуде…',
  api: 'API қызметі',
  apiDescription: 'Сервердің жауап беруін тексеру',
  database: 'Дерекқор дайындығы',
  databaseDescription: 'Байланыс пен миграцияларды тексеру',
  checking: 'Тексерілуде',
  online: 'Қолжетімді',
  ready: 'Дайын',
  unavailable: 'Қолжетімсіз',
  apiReadyNote: 'Сервер сұрауларға жауап береді.',
  databaseReadyNote: 'Дерекқор қосылған, схема дайын.',
  checkNote: 'Серверден жауап күтілуде.',
  unavailableNote: 'Қызметпен байланыс орнатылмады. Кейінірек қайта тексеріңіз.',
  live: 'Нақты уақыттағы тексеру',
  autoRefresh: 'Әр 30 секунд сайын жаңартылады',
  nextTitle: 'Алдағы мүмкіндіктер',
  nextDescription: 'Клиника қызметкерлеріне арналған қарапайым қолдау жолы.',
  planned: 'Жоспарланған',
  guidesTitle: 'Мәселені өзіңіз шешіңіз',
  guidesDescription: 'NCALayer, ЭЦҚ, принтер және басқа мәселелерге арналған қадамдық нұсқаулықтар.',
  supportTitle: 'IT маманына жүгініңіз',
  supportDescription: 'Нұсқаулық көмектеспесе, өтінім жіберіп, оның орындалуын қадағалаңыз.',
  insightsTitle: 'Қолдауды тиімді басқарыңыз',
  insightsDescription: 'Мамандарға арналған өтінімдер, білім базасы және нақты қолдау көрсеткіштері.',
  phaseThree: '3-кезең',
  phaseFour: '4-кезең',
  phaseFive: '5-кезең',
  privacyTitle: 'Клиниканың ішкі IT кеңістігі',
  privacyDescription: 'MedDesk техникалық қолдауға арналған. Пациенттердің медициналық деректерін, ЭЦҚ кілттерін және құпиясөздерді енгізбеңіз.',
  footer: 'MedDesk · Клиника командасына арналған',
  buildFooter: '5-кезең — аналитика және басқару',
  accessTitle: 'Сіздің жұмыс кеңістігіңіз',
  accessDescription: 'Жеке кабинет арқылы нұсқаулықтарды қарап, IT қолдау өтінімдерін басқара аласыз.',
  authPhase: '2-КЕЗЕҢДЕ ҚОЛЖЕТІМДІ',
  authPending: 'Жүйеге кіру дайындалуда',
  authPendingDescription: 'Қауіпсіз кіру және қызметкер, IT маманы мен әкімші рөлдері келесі кезеңде қосылады. Қазір есептік жазбалар қолжетімді емес.',
  authDisabled: 'Кіру әлі қолжетімді емес',
  backOverview: 'Шолуға оралу',
  protectedWorkspace: 'Қауіпсіз жеке кабинеттер',
  authRoles: 'Қызметкер · IT маманы · Әкімші',
  notFoundTitle: 'Бет табылмады',
  notFoundDescription: 'Бұл мекенжайда бет жоқ. Платформа шолуына оралыңыз.',
  ...phase2kk,
  ...phase3kk,
  ...phase4kk,
  ...phase5kk,
} as const

type Dictionary = { [K in keyof typeof kk]: string }
const ru: Dictionary = {
  clinic: 'IT-поддержка клиники',
  workspace: 'Рабочее пространство',
  overview: 'Обзор',
  login: 'Войти в систему',
  soon: 'СКОРО',
  knowledge: 'База знаний',
  tickets: 'Заявки в поддержку',
  analytics: 'Аналитика',
  notSignedIn: 'Вы не вошли в систему',
  internalPortal: 'Внутренний портал',
  mainNavigation: 'Основная навигация',
  openMenu: 'Открыть меню',
  closeMenu: 'Закрыть меню',
  chooseLanguage: 'Язык интерфейса',
  skip: 'Перейти к основному содержимому',
  foundation: 'ЭТАП 5 · АНАЛИТИКА И УПРАВЛЕНИЕ',
  greeting: 'IT-поддержка в одном месте.',
  greetingDescription: 'MedDesk — единое рабочее пространство клиники для решения повседневных технических проблем.',
  foundationNotice: 'Пространство для вашей команды',
  foundationDescription: 'Найдите проблему в базе знаний и выполните шаги решения. Сообщите, помогла ли инструкция. Если проблема осталась, отправьте заявку IT-специалисту.',
  currentPhase: 'Текущий этап',
  phaseOne: '05',
  phaseOneTitle: 'Аналитика и управление',
  phaseOneDescription: 'Реальные показатели, журнал действий и настройки клиники.',
  development: 'Версия в разработке',
  connectedTitle: 'Состояние системы',
  connectedDescription: 'Текущая доступность API и базы данных.',
  refresh: 'Обновить',
  refreshing: 'Проверка…',
  api: 'Сервис API',
  apiDescription: 'Проверка ответа сервера',
  database: 'Готовность базы данных',
  databaseDescription: 'Проверка подключения и миграций',
  checking: 'Проверка',
  online: 'Доступен',
  ready: 'Готова',
  unavailable: 'Недоступен',
  apiReadyNote: 'Сервер отвечает на запросы.',
  databaseReadyNote: 'База подключена, схема готова.',
  checkNote: 'Ожидаем ответ сервера.',
  unavailableNote: 'Не удалось связаться с сервисом. Повторите проверку позже.',
  live: 'Проверка в реальном времени',
  autoRefresh: 'Обновляется каждые 30 секунд',
  nextTitle: 'Возможности впереди',
  nextDescription: 'Простой путь к технической помощи для сотрудников клиники.',
  planned: 'Запланировано',
  guidesTitle: 'Решайте проблемы самостоятельно',
  guidesDescription: 'Пошаговые инструкции для NCALayer, ЭЦП, принтеров и других технических вопросов.',
  supportTitle: 'Обращайтесь к IT-специалисту',
  supportDescription: 'Если инструкция не помогла, создайте заявку и отслеживайте её выполнение.',
  insightsTitle: 'Управляйте поддержкой эффективно',
  insightsDescription: 'Заявки, база знаний и реальные показатели поддержки для специалистов.',
  phaseThree: 'Этап 3',
  phaseFour: 'Этап 4',
  phaseFive: 'Этап 5',
  privacyTitle: 'Внутреннее IT-пространство клиники',
  privacyDescription: 'MedDesk предназначен для технической поддержки. Не вводите медицинские данные пациентов, ключи ЭЦП и пароли.',
  footer: 'MedDesk · Для команды клиники',
  buildFooter: 'Этап 5 — аналитика и управление',
  accessTitle: 'Ваше рабочее пространство',
  accessDescription: 'В личном кабинете вы сможете читать инструкции и управлять заявками в IT-поддержку.',
  authPhase: 'ДОСТУПНО НА ЭТАПЕ 2',
  authPending: 'Вход в систему готовится',
  authPendingDescription: 'Безопасный вход и роли сотрудника, IT-специалиста и администратора появятся на следующем этапе. Учётные записи пока недоступны.',
  authDisabled: 'Вход пока недоступен',
  backOverview: 'Вернуться к обзору',
  protectedWorkspace: 'Безопасные личные кабинеты',
  authRoles: 'Сотрудник · IT-специалист · Администратор',
  notFoundTitle: 'Страница не найдена',
  notFoundDescription: 'По этому адресу нет страницы. Вернитесь к обзору платформы.',
  ...phase2ru,
  ...phase3ru,
  ...phase4ru,
  ...phase5ru,
}

const LocaleContext = createContext<{
  locale: Locale
  setLocale: (locale: Locale) => void
  t: Dictionary
} | null>(null)

function initialLocale(): Locale {
  try {
    return localStorage.getItem(localeStorageKey) === 'ru' ? 'ru' : 'kk'
  } catch {
    return 'kk'
  }
}

export function LocaleProvider({ children }: { children: ReactNode }) {
  const [locale, setLocale] = useState<Locale>(initialLocale)
  useEffect(() => {
    document.documentElement.lang = locale
    document.title = locale === 'kk' ? 'MedDesk · IT қолдау' : 'MedDesk · IT-поддержка'
    try {
      localStorage.setItem(localeStorageKey, locale)
    } catch {
      // Localization also works when browser storage is disabled.
    }
  }, [locale])
  return <LocaleContext.Provider value={{ locale, setLocale, t: locale === 'kk' ? kk : ru }}>{children}</LocaleContext.Provider>
}

export function useLocale() {
  const context = useContext(LocaleContext)
  if (!context) throw new Error('LocaleProvider is required')
  return context
}
