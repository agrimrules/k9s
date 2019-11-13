package view

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/derailed/k9s/internal/k8s"
	"github.com/derailed/k9s/internal/resource"
	"github.com/derailed/k9s/internal/ui"
	"github.com/derailed/k9s/internal/ui/dialog"
	"github.com/gdamore/tcell"
	"github.com/rs/zerolog/log"
	"k8s.io/client-go/tools/portforward"
)

// Container represents a container view.
type Container struct {
	*LogResource
}

// New Container returns a new container view.
func NewContainer(title string, list resource.List, path string) ResourceViewer {
	c := Container{
		LogResource: NewLogResource(title, "", list),
	}
	c.path = &path
	c.envFn = c.k9sEnv
	c.containerFn = c.selectedContainer
	c.extraActionsFn = c.extraActions
	c.enterFn = c.viewLogs
	c.colorerFn = containerColorer

	return &c
}

// Init initializes a container view.
func (c *Container) Init(ctx context.Context) {
	c.Resource.Init(ctx)
}

// Start starts the component.
func (c *Container) Start() {}

// Stop stops the component.
func (c *Container) Stop() {}

// Name returns the component name.
func (c *Container) Name() string { return "containers" }

func (c *Container) extraActions(aa ui.KeyActions) {
	c.LogResource.extraActions(aa)

	aa[ui.KeyShiftF] = ui.NewKeyAction("PortForward", c.portFwdCmd, true)
	aa[ui.KeyShiftL] = ui.NewKeyAction("Logs Previous", c.prevLogsCmd, true)
	aa[ui.KeyS] = ui.NewKeyAction("Shell", c.shellCmd, true)
	aa[tcell.KeyEscape] = ui.NewKeyAction("Back", c.backCmd, false)
	aa[ui.KeyP] = ui.NewKeyAction("Previous", c.backCmd, false)
	aa[ui.KeyShiftC] = ui.NewKeyAction("Sort CPU", c.sortColCmd(6, false), false)
	aa[ui.KeyShiftM] = ui.NewKeyAction("Sort MEM", c.sortColCmd(7, false), false)
	aa[ui.KeyShiftX] = ui.NewKeyAction("Sort CPU%", c.sortColCmd(8, false), false)
	aa[ui.KeyShiftZ] = ui.NewKeyAction("Sort MEM%", c.sortColCmd(9, false), false)
}

func (c *Container) k9sEnv() K9sEnv {
	env := c.defaultK9sEnv()

	ns, n := namespaced(*c.path)
	env["POD"] = n
	env["NAMESPACE"] = ns

	return env
}

func (c *Container) selectedContainer() string {
	return c.masterPage().GetSelectedItem()
}

func (c *Container) viewLogs(app *App, _, res, sel string) {
	status := c.masterPage().GetSelectedCell(3)
	if status == "Running" || status == "Completed" {
		c.showLogs(false)
		return
	}
	c.app.Flash().Err(errors.New("No logs available"))
}

// Handlers...

func (c *Container) shellCmd(evt *tcell.EventKey) *tcell.EventKey {
	if !c.masterPage().RowSelected() {
		return evt
	}

	c.Stop()
	{
		shellIn(c.app, *c.path, c.masterPage().GetSelectedItem())
	}
	c.Start()

	return nil
}

func (c *Container) portFwdCmd(evt *tcell.EventKey) *tcell.EventKey {
	if !c.masterPage().RowSelected() {
		return evt
	}

	sel := c.masterPage().GetSelectedItem()
	if _, ok := c.app.forwarders[fwFQN(*c.path, sel)]; ok {
		c.app.Flash().Err(fmt.Errorf("A PortForward already exist on container %s", *c.path))
		return nil
	}

	state := c.masterPage().GetSelectedCell(3)
	if state != "Running" {
		c.app.Flash().Err(fmt.Errorf("Container %s is not running?", sel))
		return nil
	}

	portC := c.masterPage().GetSelectedCell(10)
	ports := strings.Split(portC, ",")
	if len(ports) == 0 {
		c.app.Flash().Err(errors.New("Container exposes no ports"))
		return nil
	}

	var port string
	for _, p := range ports {
		log.Debug().Msgf("Checking port %q", p)
		if !isTCPPort(p) {
			continue
		}
		port = strings.TrimSpace(p)
		break
	}
	if port == "" {
		c.app.Flash().Warn("No valid TCP port found on this container. User will specify...")
		port = "MY_TCP_PORT!"
	}
	dialog.ShowPortForward(c.Pages, port, c.portForward)

	return nil
}

func (c *Container) portForward(lport, cport string) {
	co := c.masterPage().GetSelectedCell(0)
	pf := k8s.NewPortForward(c.app.Conn(), &log.Logger)
	ports := []string{lport + ":" + cport}
	fw, err := pf.Start(*c.path, co, ports)
	if err != nil {
		c.app.Flash().Err(err)
		return
	}

	log.Debug().Msgf(">>> Starting port forward %q %v", *c.path, ports)
	go c.runForward(pf, fw)
}

func (c *Container) runForward(pf *k8s.PortForward, f *portforward.PortForwarder) {
	c.app.QueueUpdateDraw(func() {
		c.app.forwarders[pf.FQN()] = pf
		c.app.Flash().Infof("PortForward activated %s:%s", pf.Path(), pf.Ports()[0])
		dialog.DismissPortForward(c.Pages)
	})

	pf.SetActive(true)
	if err := f.ForwardPorts(); err != nil {
		c.app.Flash().Err(err)
		return
	}
	c.app.QueueUpdateDraw(func() {
		delete(c.app.forwarders, pf.FQN())
		pf.SetActive(false)
	})
}

func (c *Container) backCmd(evt *tcell.EventKey) *tcell.EventKey {
	return c.app.PrevCmd(evt)
}