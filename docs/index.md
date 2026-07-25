---
title: Datum
description: Datum reconciles Linux hosts against desired state held in a Git repository.
template: landing.html
hide:
  - navigation
  - toc
---

<div class="dt-landing">

<section class="dt-hero" id="__skip">
  <div class="dt-hero__inner">
    <div class="dt-hero__copy">
      <p class="dt-hero__eyebrow">Continuous reconciliation for Linux</p>
      <h1 class="dt-hero__title">Linux hosts, reconciled against a Git repository.</h1>
      <p class="dt-hero__lede">
        Datum keeps Linux hosts matching the desired state held in a Git repository. Every
        pass reads the machine, compares it against the repository, and corrects what
        differs.
      </p>
      <div class="dt-hero__ctas">
        <a class="dt-cta dt-cta--primary" href="introduction/">Documentation</a>
        <a class="dt-cta dt-cta--secondary" href="#install">Install</a>
      </div>
      <div class="dt-hero__meta">
        <span>Alpha</span>
      </div>
    </div>
    <div class="dt-panel">
      <div class="dt-panel__bar">
        <span class="dt-panel__dots"><i></i><i></i><i></i></span>
        <span class="dt-panel__name">fleet/roles/web/nginx.yaml</span>
      </div>
      <pre><code><span class="dt-k">datum</span><span class="dt-p">:</span> v1alpha1
<span class="dt-k">type</span><span class="dt-p">:</span> <span class="dt-t">File</span>

<span class="dt-k">name</span><span class="dt-p">:</span> nginx-config

<span class="dt-k">requires</span><span class="dt-p">:</span>
  <span class="dt-p">-</span> Package&#91;nginx&#93;

<span class="dt-k">desired</span><span class="dt-p">:</span>
  <span class="dt-k">path</span><span class="dt-p">:</span> /etc/nginx/nginx.conf
  <span class="dt-k">owner</span><span class="dt-p">:</span> root
  <span class="dt-k">group</span><span class="dt-p">:</span> root
  <span class="dt-k">mode</span><span class="dt-p">:</span> <span class="dt-s">"0640"</span>
  <span class="dt-k">source</span><span class="dt-p">:</span> files/nginx.conf
  <span class="dt-k">validate</span><span class="dt-p">:</span> nginx</code></pre>
    </div>
  </div>
</section>

<section class="dt-section dt-section--features">
  <div class="dt-section__inner">
    <div class="dt-grid">
      <div class="dt-card">
        <span class="dt-card__mark" aria-hidden="true"></span>
        <h2 class="dt-card__title">Git is the only input</h2>
        <p class="dt-card__body">
          Desired state comes from a repository at a known revision and nothing else feeds
          in.
        </p>
      </div>
      <div class="dt-card">
        <span class="dt-card__mark" aria-hidden="true"></span>
        <h2 class="dt-card__title">Reading is separate from writing</h2>
        <p class="dt-card__body">
          <code>observe</code>, <code>diff</code> and <code>plan</code> stop before
          anything on the host changes.
        </p>
      </div>
      <div class="dt-card">
        <span class="dt-card__mark" aria-hidden="true"></span>
        <h2 class="dt-card__title">The same document on Debian and Fedora</h2>
        <p class="dt-card__body">
          A resource describes package state rather than <code>apt</code>. Which provider
          realises it is read from the host.
        </p>
      </div>
    </div>
  </div>
</section>

<section class="dt-section dt-section--install" id="install">
  <div class="dt-section__inner dt-install">
    <div class="dt-install__copy">
      <p class="dt-section__eyebrow">Install</p>
      <h2 class="dt-section__title">Build from source</h2>
      <p class="dt-section__lede">
        There are no releases yet. Go 1.25 or newer is the only requirement, and the source
        tree includes an example fleet to run the read-only commands against.
      </p>
      <div class="dt-hero__ctas">
        <a class="dt-cta dt-cta--primary" href="lifecycle/installation/">Installation</a>
        <a class="dt-cta dt-cta--secondary" href="https://github.com/dsgnr/datum">Source</a>
      </div>
    </div>
    <div class="dt-install__media">
      <div class="dt-panel">
        <div class="dt-panel__bar">
          <span class="dt-panel__dots"><i></i><i></i><i></i></span>
          <span class="dt-panel__name">shell</span>
        </div>
        <pre><code>git clone https://github.com/dsgnr/datum.git
<span class="dt-k">cd</span> datum
make build

<span class="dt-c"># Read the repository and the host, change nothing</span>
./bin/datum plan --host web-001 --repo examples/fleet

<span class="dt-c"># Reconcile, then report on the pass</span>
./bin/datum reconcile --host web-001 --repo examples/fleet
./bin/datum status</code></pre>
      </div>
    </div>
  </div>
</section>


</div>
