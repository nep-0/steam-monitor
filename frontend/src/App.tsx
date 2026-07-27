import { useEffect, useMemo, useState } from 'react'
import type { FormEvent, ReactNode } from 'react'
import './App.css'

type Player = { steam_id: string; nickname: string; name: string; avatar: string; state: number; game_id: number; game: string }
type Game = { name: string; seconds: number }
type Session = { steam_id: string; player: string; game: string; started_at: number; ended_at: number; seconds: number; ongoing: boolean }
type Dashboard = { total_players: number; playing: number; top_games: Game[] }
type Health = { last_poll: string; last_error: string }
type Analytics = { daily: { date: string; seconds: number }[]; players: { steam_id: string; player: string; seconds: number }[] }
type PlayerDetail = { player: Player; sessions: Session[] }
type GanttPlayer = { steam_id: string; player: string; sessions: Session[] }
type GanttData = { players: GanttPlayer[]; time_range: { start: string; end: string } }
type HeatmapData = { days: number; heatmap: Record<string, number> }

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch('/api/' + path, options)
  const body = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(body.error || response.statusText)
  return body as T
}

function duration(seconds: number): string {
  if (seconds < 60) return seconds + 's'
  if (seconds < 3600) return Math.round(seconds / 60) + 'm'
  return (seconds / 3600).toFixed(1) + 'h'
}

function dateValue(value: string | number): string {
  const date = typeof value === 'number' ? new Date(value * 1000) : new Date(value)
  return Number.isNaN(date.getTime()) ? 'Not available' : date.toLocaleString()
}

const timelineColors = ['#56a9d9', '#8fd58c', '#d1a85a', '#c77dba', '#e58f72', '#8e9de0']

function gameColor(game: string, games: string[]): string {
  const index = games.indexOf(game)
  return timelineColors[index % timelineColors.length]
}

function Panel({ title, eyebrow, children }: { title: string; eyebrow?: string; children: ReactNode }) {
  return <section className="panel">
    <div className="panel-heading">
      {eyebrow && <p className="eyebrow">{eyebrow}</p>}
      <h2>{title}</h2>
    </div>
    {children}
  </section>
}

function Empty({ text }: { text: string }) { return <p className="empty">{text}</p> }
function Stat({ label, value }: { label: string; value: number }) { return <article className="stat"><span>{label}</span><strong>{value}</strong></article> }

function App() {
  const [view, setView] = useState('dashboard')
  const [players, setPlayers] = useState<Player[]>([])
  const [dashboard, setDashboard] = useState<Dashboard>({ total_players: 0, playing: 0, top_games: [] })
  const [sessions, setSessions] = useState<Session[]>([])
  const [health, setHealth] = useState<Health>({ last_poll: '', last_error: '' })
  const [days, setDays] = useState('30')
  const [input, setInput] = useState('')
  const [nickname, setNickname] = useState('')
  const [notice, setNotice] = useState('')
  const [busy, setBusy] = useState(false)
  const [search, setSearch] = useState('')
  const [selectedPlayer, setSelectedPlayer] = useState<PlayerDetail | null>(null)
  const [analytics, setAnalytics] = useState<Analytics>({ daily: [], players: [] })

  async function refresh() {
    try {
      const results = await Promise.all([
        request<Health>('health'),
        request<Player[]>('players'),
        request<Dashboard>('dashboard?days=7'),
        request<Session[]>('sessions?days=' + days),
        request<Analytics>('analytics?days=90'),
      ])
      setHealth(results[0])
      setPlayers(results[1])
      setDashboard({ ...results[2], top_games: results[2].top_games || [] })
      setSessions(results[3] || [])
      setAnalytics(results[4] || { daily: [], players: [] })
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Unable to refresh dashboard')
    }
  }

  useEffect(() => { void refresh() }, [days])
  useEffect(() => {
    const timer = window.setInterval(() => void refresh(), 30000)
    return () => window.clearInterval(timer)
  }, [days])

  const visiblePlayers = useMemo(() => {
    const query = search.trim().toLowerCase()
    if (!query) return players
    return players.filter(player => [player.name, player.steam_id, player.game].some(value => value.toLowerCase().includes(query)))
  }, [players, search])

  async function addPlayers(event: FormEvent) {
    event.preventDefault()
    if (!input.trim()) return
    setBusy(true)
    try {
      const result = await request<{ steam_ids: string[] }>('players', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ steam_id: input, nickname }),
      })
      await request('poll', { method: 'POST' })
      setInput('')
      setNickname('')
      setNotice(result.steam_ids.length + ' player(s) added and polled.')
      await refresh()
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Unable to add players')
    } finally {
      setBusy(false)
    }
  }

  async function removePlayer(id: string) {
    if (!window.confirm('Remove this player from monitoring?')) return
    await request('players/' + encodeURIComponent(id), { method: 'DELETE' })
    await refresh()
  }

  async function showPlayer(id: string) {
    try {
      setSelectedPlayer(await request<PlayerDetail>('players/' + id + '?days=90'))
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Unable to load player detail')
    }
  }

  async function updateNickname(id: string, nickname: string) {
    try {
      await request('players/' + encodeURIComponent(id), {
        method: 'PATCH',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ nickname }),
      })
      setNotice(nickname.trim() ? 'Nickname updated.' : 'Nickname cleared.')
      await refresh()
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Unable to update nickname')
      throw error
    }
  }

  async function pollNow() {
    setBusy(true)
    try {
      await request('poll', { method: 'POST' })
      await refresh()
    } catch (error) {
      setNotice(error instanceof Error ? error.message : 'Poll failed')
    } finally {
      setBusy(false)
    }
  }

  const nav = [['dashboard', 'Dashboard'], ['players', 'Players'], ['timeline', 'Timeline'], ['activity', 'Activity'], ['heatmap', 'Heatmap'], ['settings', 'Settings']]
  return <div className="shell">
    <header className="topbar">
      <div>
        <p className="eyebrow">LOCAL STEAM TELEMETRY</p>
        <h1>Steam Monitor</h1>
        <p className="subtle">Presence, sessions, and local play history.</p>
      </div>
      <div className="top-actions">
        <span className={health.last_error ? 'health bad' : 'health'}>{health.last_error ? 'Poll error: ' + health.last_error : 'Last poll: ' + dateValue(health.last_poll)}</span>
        <button className="button primary" disabled={busy} onClick={() => void pollNow()}>{busy ? 'Working...' : 'Poll now'}</button>
      </div>
    </header>
    <div className="layout">
      <aside className="sidebar">
        <nav>{nav.map(([key, label]) => <button className={view === key ? 'nav-item selected' : 'nav-item'} key={key} onClick={() => setView(key)}>{label}</button>)}</nav>
        <p className="side-note">SQLite history remains local and follows the configured retention period.</p>
      </aside>
      <main className="content">
        {view === 'dashboard' && <Dashboard dashboard={dashboard} players={players} sessions={sessions} onRemove={removePlayer} onManage={() => setView('players')} />}
        {view === 'players' && <Players players={visiblePlayers} total={players.length} search={search} input={input} nickname={nickname} notice={notice} busy={busy} onSearch={setSearch} onInput={setInput} onNickname={setNickname} onSubmit={addPlayers} onRemove={removePlayer} onUpdateNickname={updateNickname} onSelect={showPlayer} selected={selectedPlayer} />}
        {view === 'timeline' && <Timeline days={days} setDays={setDays} />}
        {view === 'activity' && <Activity games={dashboard.top_games} sessions={sessions} players={players} analytics={analytics} />}
        {view === 'heatmap' && <Heatmap players={players} />}
        {view === 'settings' && <Settings health={health} days={days} />}
      </main>
    </div>
  </div>
}

function PlayerRow({ player, onRemove, onEdit }: { player: Player; onRemove: (id: string) => void; onEdit?: () => void }) {
  const status = player.game_id ? 'Playing ' + (player.game || 'a game') : player.state ? 'Online' : 'Offline'
  return <article className="player-row">
    <div className="avatar">{player.avatar && <img src={player.avatar} alt="" />}</div>
    <div className="player-copy"><strong>{player.name || player.steam_id}</strong><span>{player.steam_id}</span><em className={player.game_id ? 'playing' : player.state ? 'online' : ''}>{status}</em></div>
    <div className="player-actions">
      {onEdit && <button className="button secondary" onClick={event => { event.stopPropagation(); onEdit() }}>Rename</button>}
      <button className="button danger" onClick={event => { event.stopPropagation(); onRemove(player.steam_id) }}>Remove</button>
    </div>
  </article>
}

function Dashboard({ dashboard, players, sessions, onRemove, onManage }: { dashboard: Dashboard; players: Player[]; sessions: Session[]; onRemove: (id: string) => void; onManage: () => void }) {
  return <>
    <div className="page-title"><div><p className="eyebrow">OVERVIEW</p><h2>Monitoring dashboard</h2></div><button className="button secondary" onClick={onManage}>Manage players</button></div>
    <div className="stat-grid"><Stat label="Monitored players" value={dashboard.total_players} /><Stat label="Playing now" value={dashboard.playing} /><Stat label="Recent sessions" value={sessions.length} /></div>
    <div className="two-column">
      <Panel title="Live players" eyebrow="CURRENT STATUS">{players.length ? players.map(player => <PlayerRow key={player.steam_id} player={player} onRemove={onRemove} />) : <Empty text="No players are being monitored." />}</Panel>
      <Panel title="Most played games" eyebrow="LAST 7 DAYS">{dashboard.top_games.length ? dashboard.top_games.map(game => <div className="metric" key={game.name}><span>{game.name || 'Unknown game'}</span><strong>{duration(game.seconds)}</strong></div>) : <Empty text="Completed sessions will appear here." />}</Panel>
    </div>
  </>
}

function Players({ players, total, search, input, nickname, notice, busy, onSearch, onInput, onNickname, onSubmit, onRemove, onUpdateNickname, onSelect, selected }: { players: Player[]; total: number; search: string; input: string; nickname: string; notice: string; busy: boolean; onSearch: (value: string) => void; onInput: (value: string) => void; onNickname: (value: string) => void; onSubmit: (event: FormEvent) => void; onRemove: (id: string) => void; onUpdateNickname: (id: string, nickname: string) => Promise<void>; onSelect: (id: string) => void; selected: PlayerDetail | null }) {
  const [editingID, setEditingID] = useState('')
  const [editedNickname, setEditedNickname] = useState('')
  const [savingNickname, setSavingNickname] = useState(false)

  function startNicknameEdit(player: Player) {
    setEditingID(player.steam_id)
    setEditedNickname(player.nickname || '')
  }

  async function saveNickname(event: FormEvent, id: string) {
    event.preventDefault()
    setSavingNickname(true)
    try {
      await onUpdateNickname(id, editedNickname)
      setEditingID('')
    } catch {
      // The parent already presents the API error as a visible notice.
    } finally {
      setSavingNickname(false)
    }
  }

  return <>
    <div className="page-title"><div><p className="eyebrow">MONITOR LIST</p><h2>Players</h2></div></div>
    <Panel title="Add players" eyebrow="STEAM INPUT">
      <form className="add-form" onSubmit={onSubmit}>
        <label>Profile links or IDs<textarea value={input} onChange={event => onInput(event.target.value)} placeholder="SteamID64, profile URL, vanity URL, s.team link, or account ID" required /></label>
        <label>Nickname<input value={nickname} onChange={event => onNickname(event.target.value)} placeholder="Optional" /></label>
        <button className="button primary" disabled={busy}>{busy ? 'Adding...' : 'Add and poll'}</button>
      </form>
      <p className="help">Use one entry per line or separate entries with commas. Vanity URLs may require a Steam API request.</p>
      {notice && <p className="notice">{notice}</p>}
    </Panel>
    <Panel title="Monitored players" eyebrow={players.length + ' OF ' + total}>
      <label className="search-field">Search players<input value={search} onChange={event => onSearch(event.target.value)} placeholder="Name, SteamID64, or current game" /></label>
      {players.length ? players.map(player => <div key={player.steam_id} className="player-select" onClick={() => onSelect(player.steam_id)}>
        <PlayerRow player={player} onRemove={onRemove} onEdit={() => startNicknameEdit(player)} />
        {editingID === player.steam_id && <form className="nickname-editor" onSubmit={event => void saveNickname(event, player.steam_id)} onClick={event => event.stopPropagation()}>
          <label>Local nickname<input autoFocus value={editedNickname} maxLength={80} onChange={event => setEditedNickname(event.target.value)} placeholder="Leave empty to use the Steam profile name" /></label>
          <div><button className="button primary" disabled={savingNickname}>{savingNickname ? 'Saving...' : 'Save'}</button><button className="button secondary" type="button" onClick={() => setEditingID('')}>Cancel</button></div>
        </form>}
      </div>) : <Empty text={total ? 'No players match this search.' : 'No players are being monitored yet.'} />}
    </Panel>
    {selected && <Panel title={selected.player.name || selected.player.steam_id} eyebrow="PLAYER DETAIL">
      <p className="copy">{selected.sessions.length} completed session(s) in the last 90 days.</p>
      {selected.sessions.length ? selected.sessions.slice(0, 10).map(session => <div className="metric" key={session.steam_id + session.started_at}><span>{session.game || 'Unknown game'} at {dateValue(session.started_at)}</span><strong>{duration(session.seconds)}</strong></div>) : <Empty text="No completed sessions for this player." />}
    </Panel>}
  </>
}

function Timeline({ days, setDays }: { days: string; setDays: (value: string) => void }) {
  const [data, setData] = useState<GanttData>({ players: [], time_range: { start: '', end: '' } })
  const [error, setError] = useState('')
  useEffect(() => {
    request<GanttData>('gantt?days=' + days).then(setData).catch(error => setError(error instanceof Error ? error.message : 'Unable to load timeline'))
  }, [days])
  const start = new Date(data.time_range.start).getTime()
  const end = new Date(data.time_range.end).getTime()
  const range = Math.max(1, end - start)
  const games = Array.from(new Set(data.players.flatMap(player => player.sessions.map(session => session.game || 'Unknown game'))))
  return <>
    <div className="page-title timeline-title"><div><p className="eyebrow">SESSION HISTORY</p><h2>Timeline</h2></div><select className="timeline-range" value={days} onChange={event => setDays(event.target.value)}><option value="1">Today</option><option value="7">Last 7 days</option><option value="30">Last 30 days</option></select></div>
    <Panel title="Session timeline" eyebrow={days + ' DAY WINDOW'}>
      {error && <p className="notice">{error}</p>}
      {data.players.length ? <div className="gantt">
        <div className="gantt-axis"><span>{data.time_range.start && dateValue(data.time_range.start)}</span><span>{data.time_range.end && dateValue(data.time_range.end)}</span></div>
        {data.players.map(player => <div className="gantt-row" key={player.steam_id}>
          <strong title={player.steam_id}>{player.player}</strong>
          <div className="gantt-track">{player.sessions.map(session => {
            const sessionStart = Math.max(start, session.started_at * 1000)
            const sessionEnd = Math.min(end, session.ended_at * 1000)
            const left = Math.max(0, (sessionStart - start) / range * 100)
            const width = Math.max(0, (sessionEnd - sessionStart) / range * 100)
            const game = session.game || 'Unknown game'
            const title = game + ': ' + duration(session.seconds) + (session.ongoing ? ' (ongoing)' : '')
            return <span className={session.ongoing ? 'gantt-bar ongoing' : 'gantt-bar'} key={session.started_at} style={{ left: left + '%', width: width + '%', backgroundColor: gameColor(game, games) }} title={title} />
          })}</div>
        </div>)}
      </div> : <Empty text="No sessions in this window." />}
      {games.length > 0 && <div className="gantt-legend">{games.map(game => <span key={game}><i style={{ backgroundColor: gameColor(game, games) }} />{game}</span>)}<span><i className="ongoing-key" />Ongoing session</span></div>}
    </Panel>
  </>
}

function Activity({ games, sessions, players, analytics }: { games: Game[]; sessions: Session[]; players: Player[]; analytics: Analytics }) {
  const total = games.reduce((sum, game) => sum + game.seconds, 0)
  const maxDay = Math.max(1, ...analytics.daily.map(item => item.seconds))
  return <>
    <div className="page-title"><div><p className="eyebrow">ANALYTICS</p><h2>Activity</h2></div><div className="export-actions"><a className="button secondary" href="/api/export/sessions.csv?days=90">Export CSV</a><a className="button secondary" href="/api/export/sessions.json?days=90">Export JSON</a></div></div>
    <div className="stat-grid"><Stat label="Tracked play minutes" value={Math.round(total / 60)} /><Stat label="Ranked games" value={games.length} /><Stat label="Players with history" value={new Set(sessions.map(session => session.steam_id)).size} /></div>
    <Panel title="Game activity" eyebrow="COMPLETED TIME">{games.length ? games.map((game, index) => <div className="bar-row" key={game.name}><span>{index + 1}. {game.name || 'Unknown game'}</span><div className="bar-track"><div className="bar-fill" style={{ width: (total ? Math.max(4, game.seconds / total * 100) : 0) + '%' }} /></div><strong>{duration(game.seconds)}</strong></div>) : <Empty text="No completed games yet." />}</Panel>
    <Panel title="Daily activity" eyebrow="LAST 90 DAYS">{analytics.daily.length ? analytics.daily.slice(-30).map(item => <div className="bar-row" key={item.date}><span>{item.date}</span><div className="bar-track"><div className="bar-fill" style={{ width: Math.max(2, item.seconds / maxDay * 100) + '%' }} /></div><strong>{duration(item.seconds)}</strong></div>) : <Empty text="No daily activity yet." />}</Panel>
    <Panel title="Coverage" eyebrow="MONITORING INVENTORY"><p className="copy">The monitor currently tracks <strong>{players.length}</strong> public Steam profiles.</p></Panel>
  </>
}

function Heatmap({ players }: { players: Player[] }) {
  const [days, setDays] = useState('90')
  const [player, setPlayer] = useState('')
  const [data, setData] = useState<HeatmapData>({ days: 90, heatmap: {} })
  const [error, setError] = useState('')
  useEffect(() => {
    const query = 'heatmap?days=' + days + (player ? '&player=' + encodeURIComponent(player) : '')
    request<HeatmapData>(query).then(setData).catch(error => setError(error instanceof Error ? error.message : 'Unable to load heatmap'))
  }, [days, player])
  const entries = Object.entries(data.heatmap).sort(([left], [right]) => left.localeCompare(right))
  const maximum = Math.max(1, ...entries.map(([, seconds]) => seconds))
  return <>
    <div className="page-title"><div><p className="eyebrow">CALENDAR ACTIVITY</p><h2>Heatmap</h2></div><div className="filter-actions"><select value={days} onChange={event => setDays(event.target.value)}><option value="30">Last 30 days</option><option value="90">Last 90 days</option><option value="180">Last 180 days</option></select><select value={player} onChange={event => setPlayer(event.target.value)}><option value="">All players</option>{players.map(item => <option key={item.steam_id} value={item.steam_id}>{item.name || item.steam_id}</option>)}</select></div></div>
    <Panel title={player ? 'Player contribution' : 'Team activity'} eyebrow={days + ' DAY CALENDAR'}>
      {error && <p className="notice">{error}</p>}
      {entries.length ? <><div className="heatmap-grid">{entries.map(([date, seconds]) => <div className="heatmap-cell" key={date} style={{ opacity: seconds ? 0.2 + seconds / maximum * 0.8 : 0.12 }} title={date + ': ' + duration(seconds)}><span>{new Date(date + 'T00:00:00').getDate()}</span></div>)}</div><p className="help">Darker cells indicate more recorded session time. Hover a date for its total.</p></> : <Empty text="No activity was recorded in this period." />}
    </Panel>
  </>
}

function Settings({ health, days }: { health: Health; days: string }) {
  return <>
    <div className="page-title"><div><p className="eyebrow">OPERATIONS</p><h2>Settings and diagnostics</h2></div></div>
    <div className="two-column">
      <Panel title="Runtime status" eyebrow="HEALTH"><div className="detail-list"><div><span>Last poll</span><strong>{dateValue(health.last_poll)}</strong></div><div><span>Poll errors</span><strong className={health.last_error ? 'error-text' : ''}>{health.last_error || 'None reported'}</strong></div><div><span>History view</span><strong>{days} days</strong></div></div></Panel>
      <Panel title="Configuration" eyebrow="JSON FILE"><p className="copy">Settings are read from <code>config.json</code>. Change polling, timeout, proxy, database, and retention values there, then restart the binary.</p></Panel>
    </div>
  </>
}

export default App
