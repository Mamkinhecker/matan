document.querySelectorAll("[data-show-answer]").forEach((button) => {
	button.addEventListener("click", () => {
		const card = button.closest("[data-flip-card]");
		card.querySelector(".front").classList.add("hidden");
		card.querySelector(".back").classList.remove("hidden");
	});
});

document.querySelectorAll("[data-toggle]").forEach((button) => {
	button.addEventListener("click", () => {
		const target = document.querySelector(button.dataset.toggle);
		if (target) target.classList.toggle("hidden");
	});
});

document.querySelectorAll("[data-proof-game]").forEach((game) => {
	const result = game.querySelector("[data-proof-result]");
	game.querySelectorAll(".choice").forEach((choice) => {
		choice.addEventListener("click", () => {
			game.querySelectorAll(".choice").forEach((item) => {
				item.disabled = true;
				if (item.dataset.correct === "true") item.classList.add("correct");
			});
			if (choice.dataset.correct !== "true") choice.classList.add("wrong");
			result.classList.remove("hidden");
		});
	});
});

document.querySelectorAll("[data-timer]").forEach((timer) => {
	let seconds = Number(timer.dataset.timer || 0);
	const render = () => {
		const minutes = Math.floor(seconds / 60).toString().padStart(2, "0");
		const rest = (seconds % 60).toString().padStart(2, "0");
		timer.textContent = `${minutes}:${rest}`;
	};
	render();
	const interval = window.setInterval(() => {
		seconds -= 1;
		render();
		if (seconds <= 0) window.clearInterval(interval);
	}, 1000);
});

const menuToggle = document.querySelector(".menu-toggle");
const siteNav = document.querySelector("#site-nav");
const navBackdrop = document.querySelector(".nav-backdrop");

if (menuToggle && siteNav && navBackdrop) {
	const mobileMenu = window.matchMedia("(max-width: 760px)");
	const setMenuOpen = (open) => {
		const isOpen = mobileMenu.matches && open;
		siteNav.classList.toggle("is-open", isOpen);
		navBackdrop.classList.toggle("is-visible", isOpen);
		menuToggle.setAttribute("aria-expanded", String(isOpen));
		menuToggle.setAttribute("aria-label", isOpen ? "Закрыть меню" : "Открыть меню");
		siteNav.setAttribute("aria-hidden", String(mobileMenu.matches && !isOpen));
		document.body.classList.toggle("menu-open", isOpen);
	};

	menuToggle.addEventListener("click", () => {
		setMenuOpen(!siteNav.classList.contains("is-open"));
	});
	navBackdrop.addEventListener("click", () => setMenuOpen(false));
	siteNav.querySelectorAll("a").forEach((link) => {
		link.addEventListener("click", () => setMenuOpen(false));
	});
	document.addEventListener("keydown", (event) => {
		if (event.key === "Escape") setMenuOpen(false);
	});
	mobileMenu.addEventListener("change", () => setMenuOpen(false));
	setMenuOpen(false);
}

const designToggle = document.querySelector("[data-design-toggle]");
if (designToggle) {
	const updateDesignLabel = () => {
		const isNewDesign = document.documentElement.classList.contains("design-new");
		designToggle.textContent = isNewDesign ? "Старый дизайн" : "Новый дизайн";
		designToggle.setAttribute("aria-pressed", String(!isNewDesign));
	};

	designToggle.addEventListener("click", () => {
		const useNewDesign = !document.documentElement.classList.contains("design-new");
		document.documentElement.classList.toggle("design-new", useNewDesign);
		try {
			localStorage.setItem("matan-design", useNewDesign ? "new" : "classic");
		} catch (_) {
			// The selection still applies for this page if storage is unavailable.
		}
		updateDesignLabel();
	});
	updateDesignLabel();
}
