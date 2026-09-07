exec(open("_gen.py").read())

DASHBOARD = '''      <div class="dhead">
        <h1>Dashboard</h1>
        <span class="grow"></span>
        <span class="btn quiet">Add project…</span>
      </div>

      <div class="dbody">
        <div class="verdict">
          <span class="beacon"></span>
          <div>
            <div class="headline">Serving 4 domains</div>
            <div class="subline">
              2 of 3 projects running · 5 services · proxy 192.168.64.209</div>
          </div>
        </div>

        <div>
          <div class="clabel">PROJECTS</div>
          <div class="card">
            <div class="row">
              <span class="dot on"></span><span class="name">platform</span>
              <span class="chip">test</span>
              <span class="pdots"><span class="dot on"></span><span class="dot on"></span><span class="dot on"></span></span>
              <span class="grow"></span>
              <span class="meta">3 of 3 running</span>
              <span class="btn quiet">Stop</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">guidecheck</span>
              <span class="chip">test</span>
              <span class="pdots"><span class="dot on"></span><span class="dot on"></span></span>
              <span class="grow"></span>
              <span class="meta">2 of 2 running</span>
              <span class="btn quiet">Stop</span>
            </div>
            <div class="row">
              <span class="dot off"></span><span class="name">soksak</span>
              <span class="chip">test</span>
              <span class="pdots"><span class="dot off"></span><span class="dot off"></span><span class="dot off"></span><span class="dot off"></span></span>
              <span class="grow"></span>
              <span class="meta">stopped</span>
              <span class="btn">Start</span>
            </div>
          </div>
        </div>

        <div>
          <div class="clabel">ADDRESSES <span class="note">everything the proxy is serving right now</span></div>
          <div class="card">
            <div class="row">
              <span class="dot on"></span><span class="name">platform1</span>
              <span class="addr"><a href="#">https://platform1.test/</a></span>
              <span class="meta">platform</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">platform2</span>
              <span class="addr"><a href="#">https://platform2.test/</a></span>
              <span class="meta">platform</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">platform3</span>
              <span class="addr"><a href="#">https://api.platform3.test/</a></span>
              <span class="meta">platform</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">web</span>
              <span class="addr"><a href="#">https://web.test/</a></span>
              <span class="meta">guidecheck</span>
            </div>
          </div>
        </div>
      </div>'''

PROJECT = '''      <div class="dhead">
        <h1>platform</h1>
        <span class="sub">test · ~/Work/apple-container-network-test</span>
        <span class="grow"></span>
        <span class="btn quiet">Restart all</span>
        <span class="btn">Stop</span>
      </div>

      <div class="dbody">
        <div>
          <div class="clabel">SERVICES</div>
          <div class="card">
            <div class="row">
              <span class="dot on"></span><span class="name">platform1</span>
              <span class="addr"><a href="#">https://platform1.test/</a></span>
              <span class="meta">192.168.64.204</span>
              <span class="btn quiet">Logs</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">platform2</span>
              <span class="addr"><a href="#">https://platform2.test/</a></span>
              <span class="meta">192.168.64.205</span>
              <span class="btn quiet">Logs</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">platform3</span>
              <span class="addr"><a href="#">https://api.platform3.test/</a></span>
              <span class="meta">192.168.64.206</span>
              <span class="btn quiet">Logs</span>
            </div>
          </div>
        </div>

        <div>
          <div class="clabel">PROJECT</div>
          <div class="card">
            <div class="kv"><b>Domain</b>test
              <span class="faint">&nbsp;· inherited from the machine</span>
              <span class="grow"></span><span class="btn quiet">Use another…</span></div>
            <div class="kv"><b>Compose file</b>
              <span class="strong">compose.yaml</span>
              <span class="grow"></span><span class="btn quiet">Reveal</span></div>
            <div class="kv"><b>Routes</b>3 of 3 services</div>
          </div>
        </div>
      </div>'''

DOMAINS = '''      <div class="dhead">
        <h1>Domains</h1>
        <span class="sub">delegated to bay on this machine</span>
        <span class="grow"></span>
        <span class="btn">Add domain…</span>
      </div>

      <div class="dbody">
        <div>
          <div class="card">
            <div class="row">
              <span class="dot on"></span><span class="name">test</span>
              <span class="addr muted">projects without one of their own use it</span>
              <span class="chip">default</span>
              <span class="btn dim">Remove</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">devel</span>
              <span class="addr muted">delegated</span>
              <span class="btn quiet">Make default</span>
              <span class="btn quiet">Remove</span>
            </div>
          </div>
        </div>

        <div>
          <div class="clabel">HOW IT WORKS</div>
          <div class="card">
            <div class="kv"><b>Resolver</b>
              <span class="strong">/etc/resolver/test, /etc/resolver/devel → 127.0.0.1:5354</span></div>
            <div class="kv"><b>Adding one</b>asks for your password once</div>
            <div class="kv"><b>Pinned domains</b>
              <span class="strong">a domain named in a project's compose file is removed there</span></div>
          </div>
        </div>
      </div>'''

CERTS = '''      <div class="dhead">
        <h1>Certificates</h1>
        <span class="sub">issued by this machine's local authority</span>
        <span class="grow"></span>
        <span class="btn quiet">Reissue all</span>
        <span class="btn quiet">Replace authority…</span>
      </div>

      <div class="dbody">
        <div>
          <div class="clabel">AUTHORITY</div>
          <div class="card">
            <div class="row">
              <span class="dot on"></span><span class="name">bay</span>
              <span class="addr muted">trusted in your keychain</span>
              <span class="meta">valid until 2036-09-07</span>
            </div>
          </div>
        </div>

        <div>
          <div class="clabel">ISSUED</div>
          <div class="card">
            <div class="row">
              <span class="dot on"></span><span class="name">platform1.test</span>
              <span class="addr muted">platform</span>
              <span class="meta">valid until 2027-09-07</span>
              <span class="btn quiet">Reissue</span>
            </div>
            <div class="row">
              <span class="dot on"></span><span class="name">web.test</span>
              <span class="addr muted">guidecheck</span>
              <span class="meta">valid until 2027-09-07</span>
              <span class="btn quiet">Reissue</span>
            </div>
            <div class="row">
              <span class="dot warn"></span><span class="name">api.soksak.test</span>
              <span class="addr muted">no route uses it</span>
              <span class="meta">valid until 2027-09-07</span>
              <span class="btn quiet">Remove</span>
            </div>
          </div>
        </div>
      </div>'''

SETUP = '''      <div class="dhead">
        <h1>Dashboard</h1>
        <span class="grow"></span>
        <span class="btn quiet">Add project…</span>
      </div>

      <div class="dbody">
        <div class="verdict">
          <span class="beacon warn"></span>
          <div>
            <div class="headline">Not serving yet</div>
            <div class="subline">
              1 project registered · nothing running</div>
          </div>
        </div>

        <div class="banner">
          <div class="txt">
            <b>This machine cannot serve the domains yet</b>
            Delegate <em>*.test</em> to bay, and trust its
            certificate authority. It asks for your password once.
          </div>
          <span class="grow"></span>
          <span class="btn hero">Finish setup</span>
        </div>

        <div>
          <div class="clabel">PROJECTS</div>
          <div class="card">
            <div class="row">
              <span class="dot off"></span><span class="name">platform</span>
              <span class="chip">test</span>
              <span class="pdots"><span class="dot off"></span><span class="dot off"></span><span class="dot off"></span></span>
              <span class="grow"></span>
              <span class="meta">stopped</span>
              <span class="btn dim">Start</span>
            </div>
          </div>
        </div>
      </div>'''

write("Main.dc.html", "dash", DASHBOARD)
write("DashboardLight.dc.html", "dash", DASHBOARD, theme="light")
write("Project.dc.html", "platform", PROJECT)
write("ProjectLight.dc.html", "platform", PROJECT, theme="light")
write("Domains.dc.html", "domains", DOMAINS)
write("Certificates.dc.html", "certs", CERTS)
# On a machine that is not set up, only the project the user opened exists.
write("Setup.dc.html", "dash", SETUP,
      state={"dash": "warn", "domains": "warn", "certs": "warn"},
      projects=[("platform", "platform", "off", "0/3")])
