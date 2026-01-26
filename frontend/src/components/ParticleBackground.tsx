import React, { useEffect, useMemo, useState } from 'react';
import Particles, { initParticlesEngine } from '@tsparticles/react';
import { loadSlim } from '@tsparticles/slim';
import type { ISourceOptions } from '@tsparticles/engine';
import { useTheme } from '../theme';

const ParticleBackground: React.FC = () => {
  const { mode } = useTheme();
  const [init, setInit] = useState(false);

  useEffect(() => {
    initParticlesEngine(async (engine) => {
      await loadSlim(engine);
    }).then(() => {
      setInit(true);
    });
  }, []);

  const options: ISourceOptions = useMemo(() => ({
    fullScreen: {
      enable: true,
      zIndex: -1,
    },
    fpsLimit: 60,
    particles: {
      number: {
        value: 50,
        density: {
          enable: true,
          width: 1920,
          height: 1080,
        },
      },
      color: {
        value: mode === 'light' ? '#667eea' : '#00d4ff',
      },
      links: {
        enable: true,
        color: mode === 'light' ? '#667eea' : '#00d4ff',
        opacity: mode === 'light' ? 0.3 : 0.2,
        distance: 150,
        width: 1,
      },
      move: {
        enable: true,
        speed: 1,
        direction: 'none',
        random: true,
        straight: false,
        outModes: {
          default: 'bounce',
        },
      },
      opacity: {
        value: {
          min: 0.3,
          max: 0.7,
        },
        animation: {
          enable: true,
          speed: 0.5,
          sync: false,
        },
      },
      size: {
        value: {
          min: 1,
          max: 3,
        },
      },
      shape: {
        type: 'circle',
      },
    },
    interactivity: {
      events: {
        onHover: {
          enable: true,
          mode: 'grab',
        },
        onClick: {
          enable: true,
          mode: 'push',
        },
      },
      modes: {
        grab: {
          distance: 140,
          links: {
            opacity: 0.5,
          },
        },
        push: {
          quantity: 2,
        },
      },
    },
    detectRetina: true,
    background: {
      color: 'transparent',
    },
  }), [mode]);

  if (!init) {
    return null;
  }

  return (
    <Particles
      id="tsparticles"
      options={options}
    />
  );
};

export default ParticleBackground;
