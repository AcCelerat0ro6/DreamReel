import ParticleField from "./ParticleField";
import Icon from "./Icon";
import "./CreatorWelcome.css";

export default function CreatorWelcome({ onNavigate, onCreate }) {
  return <section className="welcome-banner creation-welcome">
    <ParticleField/>
    <div className="welcome-copy">
      <span className="banner-eyebrow">YOUR NEXT FRAME STARTS HERE</span>
      <h1>让想象，<em>有了画面。</em></h1>
      <p>把一瞬间的灵感，变成属于你的作品。</p>
      <button type="button" onClick={onCreate || (() => onNavigate("/create"))}>开始创作<Icon name="arrow_outward" size={16}/></button>
    </div>
    <div className="creation-art" aria-hidden="true">
      <span className="creation-orbit"/><span className="creation-orbit creation-orbit-two"/>
      <span className="creation-frame creation-frame-back"/><span className="creation-frame creation-frame-front"><Icon name="play_arrow" size={35}/></span>
      <span className="creation-star creation-star-one">✧</span><span className="creation-star creation-star-two">✧</span>
    </div>
  </section>;
}
