package cli

import (
	"context"
	"sort"
	"strings"

	"github.com/tamnd/any-cli/kit"
	"github.com/tamnd/ytb-cli/ytb"
)

// tables.go is the tool describing itself: the surfaces a record can name, the
// clients it claims to be, and the routes `ytb serve` and `ytb mcp` answer on.
//
// Every record carries surface ids and client names, and until these commands
// existed the only way to find out what "s3" or "ANDROID_VR" meant was to open
// the source. The invariant tests in ytb/ and cli/ check each table against the
// code that serves it, so a row here that nothing reads, or a read that names a
// surface no row describes, fails the build.
//
// None of the three makes a request.

func newSurfacesCmd() kit.Command {
	return kit.Command{
		Use:   "surfaces",
		Group: "read",
		Short: "The eleven surfaces a record can name, with the host each one reads",
		Long: `Print the surface table every record's "surfaces" field points into.

A surface is one way of reading YouTube, not one endpoint. s2 is the whole web
InnerTube API because browse, next, search and player answer with the same
shapes from the same host under the same client; s3 is a separate surface from
s2 despite also being /player, because asking as ANDROID gets stream URLs and
caption URLs that return bytes and asking as WEB does not.

The tier column is the only one that is about you rather than about YouTube.
Tier 0 is anonymous and is everything except s11, which is a read that carried
your cookies.

This command makes no request.`,
		Args: kit.NoArgs,
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			return EmitAll(app, ytb.SurfaceTable(), func(s ytb.SurfaceInfo) Row {
				return Row{
					Cols:  []string{"id", "constant", "host", "tier", "summary"},
					Vals:  []string{s.ID, s.Constant, s.Host, itoa(s.Tier), s.Summary},
					Value: s,
				}
			})
		},
	}
}

func newClientsCmd() kit.Command {
	return kit.Command{
		Use:   "clients",
		Group: "read",
		Short: "Every InnerTube client this tool claims to be, with its version",
		Long: `Print the clients ytb identifies as, which is what the "client" field on a
record names.

Which client asked decides what comes back, so this is not cosmetic. ANDROID_VR
is the one anonymous client that still answers /player with directly fetchable
stream URLs and no proof-of-origin token, which is why downloads use it.
WEB_REMIX is the only one music.youtube.com answers. WEB has the richest page
data and the least media.

The version numbers are the part that goes stale. YouTube checks the client
name against the user agent and the version against neither, until one day it
does, and a read that starts failing for everybody at once is usually a version
that has aged out.

This command makes no request.`,
		Args: kit.NoArgs,
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			return EmitAll(app, ytb.Clients(), func(s ytb.ClientSpec) Row {
				return Row{
					Cols: []string{"name", "num", "version", "host", "user_agent"},
					Vals: []string{s.Name, itoa(s.Num), s.Version, s.Host, s.UserAgent},
					Value: map[string]any{
						"name":       s.Name,
						"num":        s.Num,
						"version":    s.Version,
						"host":       s.Host,
						"user_agent": s.UserAgent,
						"extra":      s.Extra,
					},
				}
			})
		},
	}
}

func newRoutesCmd() kit.Command {
	return kit.Command{
		Use:   "routes",
		Group: "read",
		Short: "Every read, with its HTTP route and its MCP tool name",
		Long: `Print the operations ytb registers, one row each.

kit derives three names from one registration: the command path, the HTTP route
that ` + "`ytb serve`" + ` answers on, and the tool name ` + "`ytb mcp`" + ` publishes. This
prints all three side by side, so what the server offers can be read without
starting it.

The list is every read. The commands that write something local, download,
crawl, archive, query, export, config and auth, are not here, because none of
them is a read of YouTube and none of them is served.

This command makes no request.`,
		Args: kit.NoArgs,
		Run: func(ctx context.Context, args []string) error {
			app := appFromCtx(ctx)
			return EmitAll(app, routeTable(), func(r routeRow) Row {
				return Row{
					Cols:  []string{"command", "route", "tool", "args", "summary"},
					Vals:  []string{r.Command, r.Route, r.Tool, r.Args, r.Summary},
					Value: r,
				}
			})
		},
	}
}

// routeRow is one operation under its three names.
type routeRow struct {
	Command string `json:"command"`
	Route   string `json:"route"`
	Tool    string `json:"tool"`
	Args    string `json:"args"`
	Summary string `json:"summary"`
}

// routeTable builds the list from a throwaway app with the domain registered on
// it, which is the same thing `ytb serve` builds. Reading the live app's own
// registry would be circular, since this command is on it.
func routeTable() []routeRow {
	app := kit.New(kit.Identity{Binary: "ytb", Short: "routes"})
	(ytb.Domain{}).Register(app)

	out := make([]routeRow, 0, len(app.Ops()))
	for _, op := range app.Ops() {
		m := op.Meta()
		name, route, tool := m.Name, m.Name, m.Name
		if m.Parent != "" {
			name = m.Parent + " " + m.Name
			route = m.Parent + "/" + m.Name
			tool = m.Parent + "_" + m.Name
		}
		args := make([]string, 0, len(m.Args))
		for _, a := range m.Args {
			s := "<" + a.Name + ">"
			if a.Variadic {
				s += "..."
			}
			args = append(args, s)
		}
		out = append(out, routeRow{
			Command: name,
			Route:   "/v1/" + route,
			Tool:    tool,
			Args:    strings.Join(args, " "),
			Summary: m.Summary,
		})
	}
	// Registration order is the order the domain file happens to be written in,
	// which is not an order anyone can look something up in.
	sort.Slice(out, func(i, j int) bool { return out[i].Command < out[j].Command })
	return out
}
