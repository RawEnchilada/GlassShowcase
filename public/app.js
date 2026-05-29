const projects = [
  {
    id: "blackwood",
    title: "Blackwood Division",
    status: "Finished",
    description: "Fight supernatural creatures with your forces in this fast paced Roguelike RTS game!",
    download: "https://store.steampowered.com/app/3967040/Blackwood_Division/",
    media: [
      "assets/blackwood/capsule-750x900.png",
      "assets/blackwood/clip1.webp",
      "assets/blackwood/clip4.webp",
      "assets/blackwood/clip5.webp"
    ]
  },
  {
    id: "dirty",
    title: "Nothing Stays Dirty",
    status: "Finished",
    description: "Cleanup a spaceship after the hero has passed through, slaying all the enemies in it and breaking stuff. There might be still some surviving monsters lurking...",
    download: "games/dirty.zip",
    media: [
      "assets/dirty/banner.png",
      "assets/dirty/2.webp",
      "assets/dirty/3.webp",
      "assets/dirty/4.webp"
    ]
  },
  {
    id: "sandstorm",
    title: "Sandstorm",
    status: "Finished",
    description: "Choose your hoverbike and try to outrun the sandstorm! Dodge the obstacles and get as far as you can.",
    download: "games/sandstorm.zip",
    media: [
      "assets/sandstorm/banner.png",
      "assets/sandstorm/1.gif",
      "assets/sandstorm/2.gif"
    ]
  },
  {
    id: "squid",
    title: "Squid Brawl",
    status: "Finished",
    description: "Which squid will stay alive? Jump in a game with your friends and try to knock each other down from the island. Last squid standing wins!",
    download: "games/squidbrawl-windows.zip",
    media: [
      "assets/squid/banner.png",
      "assets/squid/banner2.png",
      "assets/squid/banner3.png",
      "assets/squid/2026-05-29_15-13-16.webp"
    ]
  },
  {
    id: "ttl",
    title: "Take The Land",
    status: "Playable",
    description: "Fight for control over the land in this tactical roguelike card game.",
    download: "games/TakeTheLandwin.zip",
    media: [
      "assets/ttl/Screenshot_20260529_151524.png",
      "assets/ttl/Screenshot_20260529_151543.png",
      "assets/ttl/2026-05-29_15-17-24.gif",
      "assets/ttl/2026-05-29_15-18-08.gif"
    ]
  },
  {
    id: "forge",
    title: "The Forge",
    status: "Tech Demo",
    description: "Choose between randomly generated wands and win a fight against your friends in this wizard arena.",
    download: "games/TheForge.zip",
    media: [
      "assets/forge/Screenshot_20260529_151943.png",
      "assets/forge/2026-05-29_15-20-56.webp",
      "assets/forge/2026-05-29_15-21-29.webp",
      "assets/forge/2026-05-29_15-22-09.webp"
    ]
  },
  {
    id: "conways",
    title: "Conways Bet",
    status: "Tech Demo",
    description: "Conway's game of a life as a casino game! Choose your attributes and place your bets, see if your cells survive the evolutionary arena.",
    download: "games/conwaysbet.zip",
    media: [
      "assets/conways/2026-05-29_14-52-02.gif",
      "assets/conways/2026-05-29_14-53-19.webp",
      "assets/conways/2026-05-29_14-54-04.gif"
    ]
  },
  {
    id: "orcwave",
    title: "Orc Wave",
    status: "Tech Demo",
    description: "Build your walls to protect the town against a fluid simulated wave of Orcs! Actually just green balls, but you get the idea.",
    download: "games/orcwave.zip",
    media: [
      "assets/orcwave/Screenshot_20260529_152413.png",
      "assets/orcwave/2026-05-29_15-25-31.webp"
    ]
  },
  {
    id: "shipbattler",
    title: "Ship Battler",
    status: "Playable",
    description: "Place and chain modules on your spaceship in this sci-fi autobattler to outsmart your enemies!",
    download: "games/shipbattler.zip",
    media: [
      "assets/shipbattler/Screenshot_20260529_153210.png",
      "assets/shipbattler/Screenshot_20260529_153537.png",
      "assets/shipbattler/2026-05-29_15-33-07.webp",
      "assets/shipbattler/2026-05-29_15-34-29.webp"
    ]
  },
  {
    id: "game426",
    title: "Game 426",
    status: "Tech Demo",
    description: "Your followers are waiting oh m'lord! Help them survive in harsh world either through force or through kindness.",
    download: "games/game426.zip",
    media: [
      "assets/game426/Screenshot_20260529_154748.png",
      "assets/game426/2026-05-29_15-48-38.webp",
      "assets/game426/2026-05-29_15-49-37.webp"
    ]
  },
  {
    id: "nexus2",
    title: "Nexus 2",
    status: "Playable",
    description: "Try to defeat Og'Kthur in this autobattler card game!",
    download: "games/nexus2.zip",
    media: [
      "assets/nexus2/Screenshot_20260529_155607.png",
      "assets/nexus2/2026-05-29_15-56-41.webp",
      "assets/nexus2/2026-05-29_15-57-04.webp"
    ]
  },
  {
    id: "shrimp",
    title: "Shrimp",
    status: "Playable",
    description: "Find the shrimp! It's worth it I swear!",
    download: "games/shrimp.zip",
    media: [
      "assets/shrimp/Screenshot_20260529_164325.png",
      "assets/shrimp/Screenshot_20260529_164540.png",
      "assets/shrimp/2026-05-29_16-43-54.webp"
    ]
  },
  {
    id: "crimson",
    title: "Crimson Ghoul",
    status: "Finished",
    description: "Defeat the evil knight that learns from your behavior!",
    download: "games/crimson.zip",
    media: [
      "assets/crimson/Screenshot_20260529_164947.png",
      "assets/crimson/2026-05-29_16-50-12.gif",
      "assets/crimson/2026-05-29_16-50-44.gif"
    ]
  },
];

const list = document.querySelector("#projectList");
const template = document.querySelector("#cardTemplate");
const exhibitSection = document.querySelector("#exhibition");
const apiBase = (window.GLASS_TOWER_API_BASE || "").replace(/\/$/, "");
let ratings = {};
const mediaCyclers = [];
const pendingRatings = new Set();
const VIEW_ROTATION_DEGREES = 52;
let currentProject = 0;
let dragStartX = 0;
let dragStartY = 0;
let isDragging = false;
let dragMode = "idle";
let dragPointerType = "mouse";
let lastWheelAt = 0;
let exhibitionInView = false;

function linkKind(url) {
  try {
    const parsed = new URL(url, window.location.href);
    if (parsed.origin === window.location.origin) return "download";
    if (parsed.hostname.includes("steampowered.com")) return "steam";
    return "external";
  } catch {
    return "download";
  }
}

function setMediaImage(item, index) {
  const src = item.project.media[index];
  item.index = index;
  item.mounted = true;
  item.frame.hidden = false;
  item.frame.classList.remove("is-portrait");
  item.image.onload = () => {
    item.frame.classList.toggle("is-portrait", item.image.naturalHeight > item.image.naturalWidth);
  };
  item.backdrop.src = src;
  item.image.src = src;
  item.image.alt = `${item.project.title} preview ${index + 1}`;
}

function clearMediaImage(item) {
  item.mounted = false;
  item.image.onload = null;
  item.image.removeAttribute("src");
  item.image.alt = "";
  item.backdrop.removeAttribute("src");
  item.frame.classList.remove("is-portrait");
  item.frame.hidden = true;
}

function setMediaMounted(item, mounted) {
  if (!item.project.media?.length) return;
  if (mounted) {
    if (!item.mounted) {
      setMediaImage(item, item.index || 0);
    }
  } else if (item.mounted) {
    clearMediaImage(item);
  }
}

function statusKey(status) {
  return status.toLowerCase().replace(/\s+/g, "-");
}

function renderCards() {
  projects.forEach((project, index) => {
    const fragment = template.content.cloneNode(true);
    const card = fragment.querySelector(".project-card");
    card.dataset.projectId = project.id;
    card.dataset.projectIndex = String(index);
    card.dataset.status = statusKey(project.status);

    const badge = fragment.querySelector(".status-badge");
    badge.textContent = project.status;

    const media = fragment.querySelector(".project-media");
    const mediaImage = media.querySelector("img:not(.project-media-backdrop)");
    const mediaBackdrop = media.querySelector(".project-media-backdrop");
    media.querySelectorAll("img").forEach((image) => {
      image.draggable = false;
      image.loading = "lazy";
      image.decoding = "async";
    });
    if (project.media?.length) {
      const cycler = { card, frame: media, image: mediaImage, backdrop: mediaBackdrop, project, index: 0, mounted: false };
      mediaCyclers.push(cycler);
    }

    fragment.querySelector("h2").textContent = project.title;
    fragment.querySelector(".project-description").textContent = project.description;

    const link = fragment.querySelector(".download-link");
    const kind = linkKind(project.download);
    link.href = project.download;
    if (kind === "download") {
      link.textContent = "Download";
      link.setAttribute("download", `${project.id}.zip`);
    } else {
      link.textContent = kind === "steam" ? "Steam" : "Open";
      link.removeAttribute("download");
      link.target = "_blank";
      link.rel = "noopener noreferrer";
      link.setAttribute("aria-label", `Open ${project.title} ${kind === "steam" ? "on Steam" : "link"}`);
    }

    card.querySelectorAll(".rating-button").forEach((button) => {
      button.addEventListener("click", () => rateProject(project.id, button.dataset.rating, card));
    });
    card.addEventListener("click", (event) => {
      if (isInteractiveElement(event.target) || !card.classList.contains("is-neighbor")) return;
      const distance = Number(card.dataset.cardDistance);
      if (distance !== 0) rotateProjects(distance);
    });

    list.appendChild(fragment);
  });

  updateExhibition();
}

function cycleMedia() {
  mediaCyclers.forEach((item) => {
    if (!item.mounted) return;
    setMediaImage(item, (item.index + 1) % item.project.media.length);
  });
}

function syncMediaMounts() {
  mediaCyclers.forEach((item) => {
    setMediaMounted(item, exhibitionInView && !item.card.hasAttribute("aria-hidden"));
  });
}

function apiUrl(path) {
  return `${apiBase}${path}`;
}

async function loadRatings() {
  try {
    const response = await fetch(apiUrl("/api/ratings"), {
      credentials: "include",
      headers: { "Accept": "application/json" }
    });
    if (!response.ok) return;
    const payload = await response.json();
    ratings = payload.ratings || {};
    applyRatings();
  } catch {
    ratings = {};
  }
}

function applyRatings() {
  [...list.children].forEach((card) => {
    const projectId = card.dataset.projectId;
    const projectRatings = ratings[projectId] || { likes: 0, dislikes: 0, mine: "" };
    card.dataset.ratingMine = projectRatings.mine || "";
    card.querySelector('[data-count="likes"]').textContent = projectRatings.likes || 0;
    card.querySelector('[data-count="dislikes"]').textContent = projectRatings.dislikes || 0;
    card.querySelectorAll(".rating-button").forEach((button) => {
      const mine = button.dataset.rating === projectRatings.mine;
      button.classList.toggle("is-mine", mine);
      button.setAttribute("aria-disabled", projectRatings.mine ? "true" : "false");
    });
  });
}

function isInteractiveElement(target) {
  return Boolean(target.closest("button, a"));
}

function animateRating(card, rating) {
  card.classList.remove("is-liked", "is-disliked");
  requestAnimationFrame(() => {
    card.classList.add(rating === "like" ? "is-liked" : "is-disliked");
  });
}

function setRatingFeedback(card, message, tone = "") {
  const feedback = card.querySelector(".rating-feedback");
  feedback.textContent = message;
  feedback.dataset.tone = tone;
}

function clearRatingFeedbackSoon(card) {
  window.setTimeout(() => {
    const feedback = card.querySelector(".rating-feedback");
    if (feedback.dataset.tone !== "error") {
      feedback.textContent = "";
      feedback.dataset.tone = "";
    }
  }, 2200);
}

async function rateProject(projectId, rating, card) {
  if (ratings[projectId]?.mine) {
    setRatingFeedback(card, `Already rated ${ratings[projectId].mine}.`, "info");
    clearRatingFeedbackSoon(card);
    return;
  }
  if (pendingRatings.has(projectId)) return;

  const previous = { ...(ratings[projectId] || { likes: 0, dislikes: 0, mine: "" }) };
  pendingRatings.add(projectId);
  card.classList.add("is-rating");
  animateRating(card, rating);
  setRatingFeedback(card, "Saving rating...", "info");
  ratings[projectId] = {
    likes: (previous.likes || 0) + (rating === "like" ? 1 : 0),
    dislikes: (previous.dislikes || 0) + (rating === "dislike" ? 1 : 0),
    mine: rating
  };
  applyRatings();

  try {
    const response = await fetch(apiUrl("/api/rate"), {
      method: "POST",
      credentials: "include",
      headers: {
        "Accept": "application/json",
        "Content-Type": "application/json"
      },
      body: JSON.stringify({ projectId, rating })
    });
    const payload = await response.json().catch(() => ({}));
    if (!response.ok && response.status !== 409) {
      throw new Error(payload.error || "rating failed");
    }
    ratings[projectId] = {
      likes: payload.likes || 0,
      dislikes: payload.dislikes || 0,
      mine: payload.mine || rating
    };
    applyRatings();
    setRatingFeedback(card, payload.accepted === false ? "Already rated." : "Rating saved.", payload.accepted === false ? "info" : "success");
    clearRatingFeedbackSoon(card);
  } catch {
    ratings[projectId] = previous;
    applyRatings();
    card.classList.remove("rating-failed");
    requestAnimationFrame(() => card.classList.add("rating-failed"));
    setRatingFeedback(card, "Couldn't save rating. Try again.", "error");
  } finally {
    pendingRatings.delete(projectId);
    card.classList.remove("is-rating");
  }
}

function wrappedProject(index) {
  return ((index % projects.length) + projects.length) % projects.length;
}

function updateExhibition() {
  const activeIndex = wrappedProject(currentProject);
  const degrees = currentProject * VIEW_ROTATION_DEGREES;

  [...list.children].forEach((card) => {
    const index = Number(card.dataset.projectIndex);
    const forward = wrappedProject(index - activeIndex);
    const signedDistance = forward > projects.length / 2 ? forward - projects.length : forward;
    const active = index === activeIndex;
    const visible = Math.abs(signedDistance) <= 1;
    card.style.setProperty("--card-offset", String(Math.max(-1, Math.min(1, signedDistance))));
    card.dataset.cardDistance = String(signedDistance);
    card.classList.toggle("is-active", active);
    card.classList.toggle("is-neighbor", visible && !active);
    card.toggleAttribute("aria-hidden", !visible);
  });
  syncMediaMounts();

  window.glassTowerRotationDeg = degrees;
  window.dispatchEvent(new CustomEvent("exhibitionrotate", {
    detail: { degrees }
  }));
}

function rotateProjects(delta) {
  currentProject += delta;
  updateExhibition();
}

function bindExhibitionControls() {
  const shell = document.querySelector(".page-shell");
  shell.addEventListener("dragstart", (event) => event.preventDefault());

  shell.addEventListener("wheel", (event) => {
    if (Math.abs(event.deltaX) <= Math.abs(event.deltaY)) return;
    event.preventDefault();
    const now = performance.now();
    if (now - lastWheelAt < 260) return;
    if (Math.abs(event.deltaX) > 10) {
      rotateProjects(event.deltaX > 0 ? 1 : -1);
      lastWheelAt = now;
    }
  }, { passive: false });

  shell.addEventListener("pointerdown", (event) => {
    if (isInteractiveElement(event.target)) return;
    dragMode = "pending";
    isDragging = false;
    dragStartX = event.clientX;
    dragStartY = event.clientY;
    dragPointerType = event.pointerType || "mouse";
  });

  shell.addEventListener("pointermove", (event) => {
    if (isInteractiveElement(event.target)) return;
    if (dragMode === "idle") return;

    const dx = event.clientX - dragStartX;
    const dy = event.clientY - dragStartY;

    if (dragMode === "pending") {
      if (Math.max(Math.abs(dx), Math.abs(dy)) < 12) return;
      if (Math.abs(dy) > Math.abs(dx)) {
        dragMode = "idle";
        return;
      }
      dragMode = "horizontal";
      isDragging = true;
      shell.setPointerCapture(event.pointerId);
    }

    if (!isDragging) return;
    event.preventDefault();
    const sensitivity = dragPointerType === "mouse" ? 180 : 88;
    const delta = dragStartX - event.clientX;
    const moved = Math.trunc(delta / sensitivity);
    if (moved !== 0) {
      rotateProjects(Math.max(-1, Math.min(1, moved)));
      dragStartX = event.clientX;
    }
  });

  shell.addEventListener("pointerup", (event) => {
    if (isDragging && shell.hasPointerCapture(event.pointerId)) {
      shell.releasePointerCapture(event.pointerId);
    }
    isDragging = false;
    dragMode = "idle";
  });

  shell.addEventListener("pointercancel", () => {
    isDragging = false;
    dragMode = "idle";
  });

  window.addEventListener("keydown", (event) => {
    if (event.key === "ArrowRight") rotateProjects(1);
    if (event.key === "ArrowLeft") rotateProjects(-1);
  });
}

function bindExhibitionVisibility() {
  const setInView = (inView) => {
    exhibitionInView = inView;
    syncMediaMounts();
  };

  if ("IntersectionObserver" in window) {
    const observer = new IntersectionObserver((entries) => {
      setInView(entries.some((entry) => entry.isIntersecting));
    }, { threshold: 0.08 });
    observer.observe(exhibitSection);
    return;
  }

  const check = () => {
    const rect = exhibitSection.getBoundingClientRect();
    setInView(rect.top < window.innerHeight * 0.92 && rect.bottom > window.innerHeight * 0.08);
  };
  window.addEventListener("scroll", check, { passive: true });
  window.addEventListener("resize", check);
  check();
}

renderCards();
bindExhibitionVisibility();
bindExhibitionControls();
loadRatings();
if (mediaCyclers.length) {
  window.setInterval(cycleMedia, 4500);
}
