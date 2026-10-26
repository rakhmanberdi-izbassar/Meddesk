import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { BookOpen, Eye, Plus, Search, ArrowUpRight, Tag } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { Pagination } from '../components/forms/Pagination'
import { KnowledgeState } from '../components/KnowledgeState'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { queryString } from '../lib/api'
import { articlesPageSchema, localizeArticle, hasStaffRole, knowledgePath } from '../lib/knowledge'
import { useCategories } from '../lib/useCategories'

export function Knowledge({ manage = false }: { manage?: boolean }) {
  const { t, locale } = useLocale()
  const { api, user } = useAuth()
  const [params, setParams] = useSearchParams()
  const [search, setSearch] = useState(params.get('q') ?? '')
  const queryText = (params.get('q') ?? '').slice(0, 100)
  const category = params.get('category_id') ?? ''
  const tag = (params.get('tag') ?? '').slice(0, 60)
  const published = manage ? params.get('published') ?? 'all' : 'true'
  const requestedPage = Number(params.get('page') ?? 0)
  const page = Number.isInteger(requestedPage) && requestedPage >= 0 && requestedPage <= 5000 ? requestedPage : 0
  const limit = 12
  const categories = useCategories()
  const articles = useQuery({ queryKey: ['knowledge', 'articles', queryText, category, tag, published, page], queryFn: ({ signal }) => api(`${knowledgePath}/articles?${queryString({ q: queryText, category_id: category, tag, published, limit, offset: page * limit })}`, articlesPageSchema, { signal }) })
  function change(values: Record<string, string>) {
    const next = new URLSearchParams(params)
    Object.entries(values).forEach(([key, value]) => { if (value) next.set(key, value); else next.delete(key) })
    setParams(next)
  }
  return <><div className="page-heading"><div><div className="eyebrow"><span />MedDesk · {t.knowledge}</div><h1>{manage ? t.knowledgeManage : queryText ? t.searchResults : t.knowledge}</h1><p>{manage ? t.knowledgeManageDescription : t.knowledgeDescription}</p></div>{hasStaffRole(user?.role) && <Button asChild className="heading-action" variant={manage ? 'default' : 'outline'}><Link to={manage ? '/engineer/knowledge/new' : '/engineer/knowledge'}>{manage ? <Plus size={17} aria-hidden="true" /> : <BookOpen size={17} aria-hidden="true" />}{manage ? t.addArticle : t.knowledgeManage}</Link></Button>}</div>
    <section className="knowledge-search content-card"><form onSubmit={event => { event.preventDefault(); change({ q: search.trim(), page: '' }) }} className="knowledge-search-form"><div className="search-input"><Search size={19} aria-hidden="true" /><input value={search} onChange={event => setSearch(event.target.value)} maxLength={100} aria-label={t.searchArticles} placeholder={t.knowledgeSearch} /></div><Button type="submit">{t.search}</Button></form><div className="knowledge-filters"><label className="sr-only" htmlFor="knowledge-category">{t.category}</label><select id="knowledge-category" value={category} onChange={event => change({ category_id: event.target.value, page: '' })} disabled={categories.isPending}><option value="">{t.allCategories}</option>{categories.data?.map(item => <option key={item.id} value={item.id}>{locale === 'kk' ? item.name_kk : item.name_ru}</option>)}</select>{manage && <><label className="sr-only" htmlFor="article-status">{t.status}</label><select id="article-status" value={published} onChange={event => change({ published: event.target.value, page: '' })}><option value="all">{t.allStatuses}</option><option value="true">{t.published}</option><option value="false">{t.draft}</option></select></>}{(queryText || category || tag) && <Button variant="ghost" onClick={() => { setSearch(''); setParams(manage ? { published } : {}) }}>{t.clearSearch}</Button>}{tag && <Badge><Tag size={13} aria-hidden="true" />{tag}</Badge>}</div>{categories.isError && <p className="inline-error" role="alert">{t.requestFailed} <Button size="sm" variant="ghost" onClick={() => void categories.refetch()}>{t.retry}</Button></p>}</section>
    <KnowledgeState pending={articles.isPending} error={articles.error} retry={() => void articles.refetch()}>{articles.data && <><div className="knowledge-results-count" aria-live="polite">{articles.data.total} · {t.knowledge}</div><div className="article-grid">{articles.data.items.map(item => localizeArticle(item, locale)).map(article => <article className="article-card" key={article.id}><div className="article-card-top"><Badge className="article-category">{locale === 'kk' ? article.category_name_kk : article.category_name_ru}</Badge>{manage && <Badge className={article.is_published ? 'active-status' : 'inactive-status'}>{article.is_published ? t.published : t.draft}</Badge>}</div><h2><Link to={`/knowledge/articles/${article.id}`}>{article.title}</Link></h2><p>{article.description}</p><div className="article-tags">{article.tags.slice(0, 4).map(value => <button key={value} onClick={() => change({ tag: value, page: '' })}>#{value}</button>)}</div><div className="article-card-footer"><span><Eye size={14} aria-hidden="true" />{article.view_count}</span><Link to={manage ? `/engineer/knowledge/${article.id}/edit` : `/knowledge/articles/${article.id}`}>{manage ? t.edit : t.openArticle}<ArrowUpRight size={14} aria-hidden="true" /></Link></div></article>)}</div>{articles.data.items.length === 0 && <div className="table-state content-card"><BookOpen size={32} aria-hidden="true" /><p>{t.noArticles}</p></div>}<Pagination page={page} limit={limit} total={articles.data.total} busy={articles.isFetching} onChange={next => change({ page: String(next) })} /></>}</KnowledgeState>
  </>
}
