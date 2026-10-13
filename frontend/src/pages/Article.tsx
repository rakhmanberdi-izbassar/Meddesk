import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeft, CheckCircle2, Eye, Pencil, ThumbsDown, ThumbsUp } from 'lucide-react'
import { Button } from '../components/ui/button'
import { Badge } from '../components/ui/badge'
import { KnowledgeState } from '../components/KnowledgeState'
import { useAuth } from '../lib/auth'
import { useLocale } from '../lib/locale'
import { useErrorMessage } from '../lib/notifications'
import { ApiError } from '../lib/api'
import { articleResultSchema, feedbackSchema, hasStaffRole, knowledgePath, mediaURL, viewsSchema, type Article as ArticleData } from '../lib/knowledge'

function ArticleContent({ article }: { article: ArticleData }) {
  const { t, locale } = useLocale()
  const { api, user } = useAuth()
  const message = useErrorMessage()
  const viewed = useRef(false)
  const attempt = useRef(crypto.randomUUID())
  const [choice, setChoice] = useState<boolean | null>(null)
  const [recorded, setRecorded] = useState(false)
  const [busy, setBusy] = useState(false)
  const [failure, setFailure] = useState<unknown>(null)
  const [views, setViews] = useState(article.view_count)
  useEffect(() => {
    if (!article.is_published || !article.category_active || viewed.current) return
    viewed.current = true
    void api(`${knowledgePath}/articles/${article.id}/views`, viewsSchema, { method: 'POST', body: {} }).then(result => setViews(result.view_count)).catch(() => { /* Viewing still works if counter recording is unavailable. */ })
  }, [api, article.id, article.is_published, article.category_active])
  const answer = async (helpful: boolean) => {
    setChoice(helpful); setBusy(true); setFailure(null)
    try {
      await api(`${knowledgePath}/articles/${article.id}/feedback`, feedbackSchema, { method: 'POST', body: { attempt_id: attempt.current, helpful } })
      setRecorded(true)
    } catch (error) { setFailure(error) }
    finally { setBusy(false) }
  }
  return <><div className="article-navigation"><Link to="/knowledge"><ArrowLeft size={16} aria-hidden="true" />{t.backKnowledge}</Link>{hasStaffRole(user?.role) && <Button asChild variant="outline" size="sm"><Link to={`/engineer/knowledge/${article.id}/edit`}><Pencil size={14} aria-hidden="true" />{t.edit}</Link></Button>}</div><article className="article-detail content-card"><header><div className="article-card-top"><Badge>{locale === 'kk' ? article.category_name_kk : article.category_name_ru}</Badge>{!article.is_published && <Badge className="inactive-status">{t.draft}</Badge>}</div><h1>{article.title}</h1><p className="article-summary">{article.description}</p><div className="article-metadata"><span>{t.author}: {article.author_name}</span><span>{t.updated}: {new Date(article.updated_at).toLocaleDateString(locale === 'kk' ? 'kk-KZ' : 'ru-RU')}</span><span><Eye size={14} aria-hidden="true" />{views} {t.views}</span></div></header><section><h2>{t.symptoms}</h2><p className="preserve-lines">{article.symptoms}</p></section><section><h2>{t.solution}</h2><ol className="solution-steps">{article.solution_steps.map((step, index) => <li key={index}><span>{index + 1}</span><p>{step}</p></li>)}</ol></section>{article.video_id && <section><h2>{t.video}</h2><video className="article-video" controls preload="metadata" aria-label={t.video} src={mediaURL(article.video_id)} /></section>}{article.image_ids.length > 0 && <section><h2>{t.screenshots}</h2><div className="article-images">{article.image_ids.map((id, index) => <a key={id} href={mediaURL(id)} target="_blank" rel="noopener noreferrer"><img src={mediaURL(id)} loading="lazy" alt={`${t.screenshots} ${index + 1}: ${article.title}`} /></a>)}</div></section>}<div className="article-tags">{article.tags.map(tag => <Link key={tag} to={`/knowledge?tag=${encodeURIComponent(tag)}`}>#{tag}</Link>)}</div></article>{article.is_published && article.category_active && <section className="article-feedback content-card" aria-labelledby="helpful-question"><h2 id="helpful-question">{t.helpfulQuestion}</h2>{recorded ? <><p className="feedback-confirmation" role="status"><CheckCircle2 size={20} aria-hidden="true" />{choice ? t.helpfulResolved : t.helpfulUnresolved}</p>{choice === false && <Button asChild><Link to={`/tickets/new?article_id=${encodeURIComponent(article.id)}`} state={{ title: article.title, description: article.symptoms, category_id: article.category_id }}>{t.newTicket}</Link></Button>}</> : <><div className="feedback-buttons"><Button variant="outline" disabled={busy || choice !== null} onClick={() => void answer(true)}><ThumbsUp size={17} aria-hidden="true" />{t.helpfulYes}</Button><Button variant="outline" disabled={busy || choice !== null} onClick={() => void answer(false)}><ThumbsDown size={17} aria-hidden="true" />{t.helpfulNo}</Button></div>{busy && <p role="status">{t.saving}</p>}{failure != null && <div className="inline-error" role="alert">{message(failure)} <Button variant="ghost" size="sm" onClick={() => void answer(choice!)} disabled={busy}>{t.feedbackRetry}</Button></div>}</>}</section>}</>
}

export function Article() {
  const { id = '' } = useParams()
  const { api } = useAuth()
  const { t } = useLocale()
  const article = useQuery({ queryKey: ['knowledge', 'article', id], queryFn: ({ signal }) => api(`${knowledgePath}/articles/${encodeURIComponent(id)}`, articleResultSchema, { signal }) })
  if (article.error instanceof ApiError && article.error.status === 404) return <div className="table-state"><p role="alert">{t.articleMissing}</p><Button asChild variant="outline"><Link to="/knowledge">{t.backKnowledge}</Link></Button></div>
  return <KnowledgeState pending={article.isPending} error={article.error} retry={() => void article.refetch()}>{article.data && <ArticleContent key={article.data.article.id} article={article.data.article} />}</KnowledgeState>
}
