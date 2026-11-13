# Jobs

Status: implemented.

A job is a run of a Compose service that ends: a test suite, a build, a
migration, a report. A machine runs one job at a time, in the order jobs were
queued, so two heavy runs never compete for its memory. Jobs are queued on
request or on a schedule, and each one's output, exit status, duration and peak
memory are kept.

## Declaring a job

A job is a service with `x-bay.job: true`. `up` and `start` leave it alone, no
service may depend on it, and it serves no domain.

```yaml
name: orm

services:
  mysql:
    image: mysql:8.4
    expose: ["3306"]
    environment:
      MYSQL_ROOT_PASSWORD: dev
    healthcheck:
      test: ["CMD", "mysqladmin", "ping", "-h", "127.0.0.1"]
      interval: 2s
      retries: 30

  test:
    image: rust:1
    working_dir: /src
    volumes: ["./:/src"]
    command: ["cargo", "test", "-j", "2"]
    mem_limit: 2g
    environment:
      DATABASE_URL: mysql://root:dev@orm-mysql.bay.test:3306/test
    x-bay:
      job: true
      schedule: "0 3 * * *"   # optional: every day at 03:00, machine time
      timeout: 45m            # optional: the limit on one run, 1h by default
```

| Key | Default | Meaning |
| --- | --- | --- |
| `x-bay.job` | `false` | The service is a job. |
| `x-bay.schedule` | none | Five cron fields (minute, hour, day of month, month, day of week) in the machine's local time, or `@hourly`, `@daily`, `@weekly`, `@monthly`, `@yearly`. Each field takes `*`, a number, `a-b`, `a,b` and `/n`. As in cron, when both day fields are restricted a day matching either runs. |
| `x-bay.timeout` | `1h` | The longest one run may take, at most `24h`. |

A wrong value is refused when the file is read, by name.

## What a run does

The agent (`bayd`, the DNS agent on macOS and `bay-agent.service` on Linux)
takes the oldest queued job when none runs, and runs `bay job run <id>` with its
output going to the job's log. That run:

1. brings the project's other services up, as `up` does, with their health
   checks and certificates;
2. creates the job's container and starts it attached, writing its output to
   the log as it comes;
3. records the container's exit status, how long it ran and, on Linux with
   Docker, the most memory it used (`memory.peak` of its control group);
4. removes the job's container;
5. takes the project down again when it had no container before the job, and
   leaves it as it was when it did.

A job at its limit, or one cancelled, is stopped with its `stop_signal` and
`stop_grace_period`, as `stop` stops a service, and still goes through 4 and 5.
It is recorded `failed` with `stopped at its time limit` or `cancelled`. If the
run's process does not end within five minutes of the limit, the agent ends it.

A run is a process of its own, in its own process group, so it outlives the
agent: an agent that is updated or restarted leaves the job running, and the
agent that starts next takes it over and waits for it (`taking over its run,
process <pid>` in its log). On Linux the agent's unit has `KillMode=process`
for this, which `bay install` writes; `check` reports a unit written before.
An agent that finds a job recorded running whose process is gone records it
`failed` with `its run ended without recording a result`.

## Commands

```sh
bay job add test               # queue a run; prints its ID
bay job add -key "$COMMIT" test  # queue once per key; the same key prints the first ID
bay job add -wait test         # queue, print the output as it comes, exit 0 or 1
bay job list                   # the latest 50 jobs and the followed schedules
bay job list --json
bay job show <id>              # one job, with the path of its output
bay job logs [-f] <id>         # its output; -f follows it until the job ends
bay job wait [-timeout 2h] <id>  # exit 0 when it succeeded, 1 when it did not
bay job cancel <id>            # drop a queued job, or stop a running one
bay job schedule               # follow this project's schedules
bay job unschedule             # stop following them
bay job list -peer odroid      # an approved peer's jobs, read over the link
bay job show -peer odroid <id>
bay job logs -peer odroid <id> # the end of its output
bay job wait -peer odroid <id> # wait across a dropped link
```

`-key` names what a run is for, such as a commit. A job with the key already
queued, running or ended among the kept jobs is not queued again: `add` prints
that job's ID, says so on standard error, and `add -wait` waits for it. A
caller that sends `add` again after a dropped connection gets one run.

An ID can be shortened to a part that names one job, from its start or its
end. Every wait states its bound before it begins: `-wait`, `wait` and
`logs -f` wait at most `-timeout`, two hours unless given, and say so on
standard error. A wait that reaches its bound exits 1 and leaves the job as it
is; `bay job wait <id>` waits again.

`job add` and `job schedule` warn on standard error when the agent is not
running, because nothing then takes the job. `bay check` names what is missing.

## Schedules

`bay job schedule` records the project's Compose file in
`<state>/jobs/schedules.json`; from then on the agent reads the file each
minute and queues each job whose schedule names that minute. A schedule that
comes round while its last job is queued or running queues nothing, so a
schedule faster than the machine does not fill the queue. The file is read
again each minute, so a changed schedule applies without running `schedule`
again. A file that cannot be read is reported in `job list` and skipped.

## Where jobs are kept

`<state>/jobs/` holds one `<id>.json` per job and its output in `<id>.log`. The
latest 200 ended jobs are kept, with their output. The queue is changed under a
lock in `<state>/locks/`, so commands run at once keep each other's jobs.

## Another machine's jobs

A job runs on the machine that holds the project, because the project's files,
volumes and setup are there. A job on another machine is queued there:

```sh
ssh odroid bay -f /srv/orm/compose.yaml job add -key "$COMMIT" test
bay job wait -peer odroid <id>
```

The network between the machines can end at any time; the job does not depend
on it. Once queued, the job runs to its end on its machine, and its result and
output stay there until they are read. `wait -peer` reads the peer's jobs over
the link every five seconds and, when the link is gone, says so once and tries
again until its bound, so a link that comes back delivers the result. A caller
whose `add` was cut off before it printed the ID sends it again with the same
`-key` and gets the same job.

A machine tells its approved peers about its jobs as it tells them its load:
the agent writes `jobs.json` beside the peer document every five seconds, with
the latest 50 jobs and the end of the output (2 KiB) of the latest 10, and the
link serves it at `/__bay/jobs` only to a client whose certificate an approved
authority issued.

## Notifications

On macOS the agent shows a notification when a job ends, on this machine or on
an approved peer, which it reads every 15 seconds: the project, the job, its
state, how long it ran and, when it failed, its exit status or reason. A job
of a peer that ended while the link was down is announced when the link is
back; what was announced is kept in `<state>/jobs/announced.json`, so a
restarted agent announces nothing twice. A Linux machine shows none; the Macs
that approve it do.

## For an agent

Over MCP, `job` is a tool like every command. Queue with
`args ["add", "test"]`, which returns the ID at once, and then call
`args ["wait", "<id>"]`; or queue and wait in one call with
`args ["add", "-wait", "test"]`. The result is the job's output and an exit
status of 0 or 1. Run heavy work on a small machine as a job rather than with
`up` and `exec`, so it waits its turn.
