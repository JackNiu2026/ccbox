// Stroke icons drawn on a 24×24 grid; each entry is a list of path data.
export const icons = {
  network: ['M12 3a9 9 0 1 0 0 18 9 9 0 0 0 0-18z', 'M3.6 9h16.8', 'M3.6 15h16.8', 'M12 3a14 14 0 0 1 0 18', 'M12 3a14 14 0 0 0 0 18'],
  fingerprint: ['M12 11v3a8 8 0 0 1-1.5 4.7', 'M8.5 15.5A12 12 0 0 0 9 11a3 3 0 0 1 6 0v1', 'M5.6 17.4A11 11 0 0 0 6 11a6 6 0 0 1 10-4.5', 'M18 12c0 3-.5 5.6-1.3 7.6', 'M14.6 19.9c.5-1.3.9-2.8 1.1-4.4'],
  link: ['M10 14a4.5 4.5 0 0 0 6.4 0l3-3a4.5 4.5 0 0 0-6.4-6.4l-1 1', 'M14 10a4.5 4.5 0 0 0-6.4 0l-3 3a4.5 4.5 0 0 0 6.4 6.4l1-1'],
  shield: ['M12 3 4 6v6c0 5 3.4 8.4 8 9 4.6-.6 8-4 8-9V6l-8-3z', 'm8.5 12 2.5 2.5 4.5-5'],
  window: ['M3 5h18v14H3z', 'M3 9h18', 'M6 7h.01', 'M8.5 7h.01', 'M8 13l2 2-2 2', 'M13 17h3'],
  monitor: ['M3 4h18v12H3z', 'M8 20h8', 'M12 16v4'],
  folder: ['M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z', 'M9 13h6'],
  pulse: ['M3 12h4l2.5-6 5 12 2.5-6h4'],
  disk: ['M5 4h11l3 3v13H5z', 'M8 4v5h7V4', 'M8 20v-6h8v6'],
  users: ['M16 20v-1.5a3.5 3.5 0 0 0-3.5-3.5h-5A3.5 3.5 0 0 0 4 18.5V20', 'M10 11.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7z', 'M20 20v-1.5a3.5 3.5 0 0 0-2.5-3.3', 'M15.5 4.6a3.5 3.5 0 0 1 0 6.8'],
  phone: ['M7 3h10v18H7z', 'M11 18h2'],
  server: ['M4 4h16v6H4z', 'M4 14h16v6H4z', 'M8 7h.01', 'M8 17h.01'],
  mesh: ['M6 6a2 2 0 1 0 0 .01', 'M18 6a2 2 0 1 0 0 .01', 'M12 18a2 2 0 1 0 0 .01', 'M7.5 7.5l3.5 8.5', 'M16.5 7.5 13 16', 'M8 6h8'],
  alert: ['M12 9v4', 'M12 17h.01', 'M10.3 3.9 1.8 18a2 2 0 0 0 1.7 3h17a2 2 0 0 0 1.7-3L13.7 3.9a2 2 0 0 0-3.4 0z'],
  book: ['M4 5a2 2 0 0 1 2-2h13v16H6a2 2 0 0 0-2 2z', 'M4 19V5', 'M9 7h6'],
  check: ['m5 12.5 4.5 4.5L19 7.5'],
  arrow: ['M5 12h14', 'm13 6 6 6-6 6'],
  copy: ['M9 9h11v11H9z', 'M5 15H4V4h11v1'],
  code: ['m8 8-4 4 4 4', 'm16 8 4 4-4 4', 'm14 4-4 16'],
} as const;

export type IconName = keyof typeof icons;

// The README lists four principles, nine uses and three steps in a fixed order.
export const principleIcons: IconName[] = ['network', 'fingerprint', 'link', 'shield'];
export const featureIcons: IconName[] = ['window', 'monitor', 'folder', 'pulse', 'disk', 'users', 'phone', 'server', 'mesh'];
export const stepIcons: IconName[] = ['pulse', 'monitor', 'window'];
