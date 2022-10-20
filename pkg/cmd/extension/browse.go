package extension

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/cli/cli/v2/git"
	"github.com/cli/cli/v2/internal/config"
	"github.com/cli/cli/v2/internal/ghrepo"
	"github.com/cli/cli/v2/pkg/cmd/repo/view"
	"github.com/cli/cli/v2/pkg/extensions"
	"github.com/cli/cli/v2/pkg/search"
	"github.com/spf13/cobra"
)

var appStyle = lipgloss.NewStyle().Padding(1, 2)
var sidebarStyle = lipgloss.NewStyle()

type readmeGetter interface {
	Get(string) (string, error)
}

type cachingReadmeGetter struct {
	client *http.Client
	cache  map[string]string
}

func newReadmeGetter(client *http.Client) readmeGetter {
	return &cachingReadmeGetter{
		client: client,
		cache:  map[string]string{},
	}
}

func (g *cachingReadmeGetter) Get(repoFullName string) (string, error) {
	if readme, ok := g.cache[repoFullName]; ok {
		return readme, nil
	}
	repo, err := ghrepo.FromFullName(repoFullName)
	readme, err := view.RepositoryReadme(g.client, repo, "")
	if err != nil {
		return "", err
	}
	g.cache[repoFullName] = readme.Content
	return readme.Content, nil
}

type uiModel struct {
	sidebar      sidebarModel
	extList      extListModel
	logger       *log.Logger
	readmeGetter readmeGetter
}

func newUIModel(opts extBrowseOpts, extEntries []extEntry) uiModel {
	return uiModel{
		extList:      newExtListModel(opts, extEntries),
		sidebar:      newSidebarModel(opts.logger),
		logger:       opts.logger,
		readmeGetter: opts.rg,
	}
}

func (m uiModel) Init() tea.Cmd {
	// TODO the docs say not to do this but the example code in bubbles does:
	return tea.EnterAltScreen
}

func (m uiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Printf("%#v", msg)

	var cmds []tea.Cmd
	var cmd tea.Cmd
	var newModel tea.Model

	newModel, cmd = m.extList.Update(msg)
	cmds = append(cmds, cmd)
	m.extList = newModel.(extListModel)

	item := newModel.(extListModel).SelectedItem()

	if item != nil {
		ee := item.(extEntry)
		readme, err := m.readmeGetter.Get(ee.FullName)
		if err != nil {
			ee.Readme = "could not fetch readme x_x"
			m.logger.Println(err.Error())
		} else {
			renderer, err := glamour.NewTermRenderer(
				glamour.WithAutoStyle(),
				glamour.WithWordWrap(100),
			)
			if err != nil {
				ee.Readme = "could not render readme x_x"
				m.logger.Println(err.Error())
			} else {
				ee.Readme, err = renderer.Render(readme)
				if err != nil {
					ee.Readme = "could not render readme x_x"
					m.logger.Println(err.Error())
				}
			}
		}
		m.sidebar.Content = ee.Readme
	}

	newModel, cmd = m.sidebar.Update(msg)
	cmds = append(cmds, cmd)
	m.sidebar = newModel.(sidebarModel)

	return m, tea.Batch(cmds...)
}

func (m uiModel) View() string {
	return lipgloss.JoinHorizontal(lipgloss.Top, m.extList.View(), m.sidebar.View())
}

type managerModel struct {
	logger  *log.Logger
	content string
	spinner spinner.Model
}

func newManagerModel(l *log.Logger) managerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	return managerModel{
		logger:  l,
		spinner: s,
	}
}

func (m managerModel) Install(repoFullName string) {
	// TODO
}

func (m managerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m managerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Printf("%#v", msg)
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m managerModel) View() string {
	// TODO punting on spinner for now; it just would not animate
	//return fmt.Sprintf("%s installing...", m.spinner.View())
	return m.content
}

type sidebarModel struct {
	logger   *log.Logger
	Content  string
	viewport viewport.Model
	ready    bool
}

func newSidebarModel(l *log.Logger) sidebarModel {
	// TODO
	return sidebarModel{
		logger: l,
	}
}

func (m sidebarModel) Init() tea.Cmd {
	return nil
}

func (m sidebarModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	m.logger.Printf("%#v", msg)
	// TODO
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if !m.ready {
			m.viewport = viewport.New(80, msg.Height)
			m.viewport.SetContent(m.Content)
			m.ready = true
		} else {
			m.viewport.SetContent(m.Content)
			m.viewport.Height = msg.Height
		}
	default:
		m.viewport.SetContent(m.Content)
	}

	newvp, cmd := m.viewport.Update(msg)
	m.viewport = newvp
	return m, cmd
}

func (m sidebarModel) View() string {
	return m.viewport.View()
	//return sidebarStyle.Render(m.viewport.View())
}

type extEntry struct {
	URL         string
	Owner       string
	Name        string
	FullName    string
	Readme      string
	Stars       int
	Installed   bool
	Official    bool
	description string
}

func (e extEntry) Title() string {
	installedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#62FF42"))
	officialStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#F2DB74"))

	var installed string
	var official string

	if e.Installed {
		installed = installedStyle.Render(" [installed]")
	}

	if e.Official {
		official = officialStyle.Render(" [official]")
	}

	return fmt.Sprintf("%s%s%s", e.FullName, official, installed)
}

func (e extEntry) Description() string { return e.description }
func (e extEntry) FilterValue() string { return e.Title() }

type keyMap struct {
	install key.Binding
	remove  key.Binding
	web     key.Binding
}

func newKeyMap() *keyMap {
	return &keyMap{
		install: key.NewBinding(
			key.WithKeys("i"),
			key.WithHelp("i", "install"),
		),
		remove: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "remove"),
		),
		web: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "open on web"),
		),
	}
}

type extListModel struct {
	list    list.Model
	keys    *keyMap
	manager tea.Model
	logger  *log.Logger
	browser ibrowser
}

func newExtListModel(opts extBrowseOpts, extEntries []extEntry) extListModel {
	items := make([]list.Item, len(extEntries))
	for i := range items {
		items[i] = extEntries[i]
	}
	list := list.New(items, list.NewDefaultDelegate(), 0, 0)

	keys := newKeyMap()
	list.Title = "gh extensions"
	list.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			keys.remove,
			keys.install,
			keys.web,
		}
	}

	return extListModel{
		logger:  opts.logger,
		manager: newManagerModel(opts.logger),
		list:    list,
		keys:    keys,
		browser: opts.browser,
	}
}

func (m extListModel) Init() tea.Cmd {
	return nil
}

func (m extListModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		_, h := appStyle.GetFrameSize()
		m.list.SetSize(msg.Width-100, msg.Height-h)
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		item := m.SelectedItem()
		ee := item.(extEntry)
		switch {
		case key.Matches(msg, m.keys.web):
			if err := m.browser.Browse(ee.URL); err != nil {
				m.logger.Printf("failed to open '%s': %s", ee.URL, err.Error())
			}
		case key.Matches(msg, m.keys.install):
			m.manager.(managerModel).Install(ee.FullName)
		case key.Matches(msg, m.keys.remove):
			panic("REMOVE!")
		}
	}

	var cmds []tea.Cmd

	nlm, cmd := m.list.Update(msg)
	m.list = nlm
	cmds = append(cmds, cmd)

	nfm, cmd := m.manager.Update(msg)
	m.manager = nfm

	return m, tea.Batch(cmds...)
}

func (m extListModel) SelectedItem() list.Item {
	m.logger.Printf("%#v", m.list.SelectedItem())
	return m.list.SelectedItem()
}

func (m extListModel) View() string {
	return appStyle.Render(m.list.View()) + "\n" + m.manager.View()
}

type ibrowser interface {
	Browse(string) error
}

type extBrowseOpts struct {
	cmd      *cobra.Command
	browser  ibrowser
	searcher search.Searcher
	em       extensions.ExtensionManager
	client   *http.Client
	logger   *log.Logger
	cfg      config.Config
	rg       readmeGetter
}

func extBrowse(opts extBrowseOpts) error {
	// TODO support turning debug mode on/off
	f, err := os.CreateTemp("/tmp", "extBrowse-*.txt")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())

	opts.logger = log.New(f, "", log.Lshortfile)

	// TODO spinner
	// TODO get manager to tell me what's installed so I can cross ref
	installed := opts.em.List()

	result, err := opts.searcher.Repositories(search.Query{
		Kind:  search.KindRepositories,
		Limit: 1000,
		Qualifiers: search.Qualifiers{
			Topic: []string{"gh-extension"},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to search for extensions: %w", err)
	}

	host, _ := opts.cfg.DefaultHost()

	extEntries := []extEntry{}

	for _, repo := range result.Items {
		ee := extEntry{
			URL:         "https://" + host + "/" + repo.FullName,
			FullName:    repo.FullName,
			Owner:       repo.Owner.Login,
			Name:        repo.Name,
			Stars:       repo.StargazersCount,
			description: repo.Description,
		}
		for _, v := range installed {
			// TODO consider a Repo() on Extension interface
			var installedRepo string
			if u, err := git.ParseURL(v.URL()); err == nil {
				if r, err := ghrepo.FromURL(u); err == nil {
					installedRepo = ghrepo.FullName(r)
				}
			}
			if repo.FullName == installedRepo {
				ee.Installed = true
			}
		}
		if ee.Owner == "cli" || ee.Owner == "github" {
			ee.Official = true
		}

		extEntries = append(extEntries, ee)
	}

	opts.rg = newReadmeGetter(opts.client)

	return tea.NewProgram(newUIModel(opts, extEntries)).Start()
}