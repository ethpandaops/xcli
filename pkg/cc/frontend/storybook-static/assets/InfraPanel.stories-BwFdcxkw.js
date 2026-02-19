import{j as e}from"./jsx-runtime-D_zvdyIk.js";import{f as h}from"./fixtures-CTg5x2hB.js";const c={clickhouse:"CH",redis:"RD",unknown:"??"};function g({infrastructure:r}){return r.length===0?e.jsxs("div",{className:"rounded-sm border border-border bg-surface-light p-4",children:[e.jsx("h3",{className:"mb-2 text-sm/5 font-semibold text-text-tertiary",children:"Infrastructure"}),e.jsx("p",{className:"text-xs/4 text-text-disabled",children:"No infrastructure found"})]}):e.jsxs("div",{className:"rounded-sm border border-border bg-surface-light p-4",children:[e.jsx("h3",{className:"mb-3 text-sm/5 font-semibold text-text-tertiary",children:"Infrastructure"}),e.jsx("div",{className:"flex flex-col gap-2",children:r.map(s=>e.jsxs("div",{className:"flex items-center justify-between rounded-xs bg-surface px-3 py-2",children:[e.jsxs("div",{className:"flex items-center gap-2",children:[e.jsx("span",{className:"rounded-xs bg-surface-lighter px-1.5 py-0.5 font-mono text-xs/4 text-text-tertiary",children:c[s.type]??c.unknown}),e.jsx("span",{className:"text-sm/5 text-text-secondary",children:s.name})]}),e.jsx("span",{className:`text-xs/4 font-medium ${s.status==="running"?"text-success":"text-text-muted"}`,children:s.status})]},s.name))})]})}g.__docgenInfo={description:"",methods:[],displayName:"InfraPanel",props:{infrastructure:{required:!0,tsType:{name:"Array",elements:[{name:"InfraInfo"}],raw:"InfraInfo[]"},description:""}}};const j={title:"Components/InfraPanel",component:g,decorators:[r=>e.jsx("div",{className:"bg-bg p-8",children:e.jsx(r,{})})]},t={args:{infrastructure:h}},n={args:{infrastructure:[{name:"clickhouse-cbt",status:"running",type:"clickhouse"},{name:"redis",status:"stopped",type:"redis"}]}},a={args:{infrastructure:[]}};var o,i,u;t.parameters={...t.parameters,docs:{...(o=t.parameters)==null?void 0:o.docs,source:{originalSource:`{
  args: {
    infrastructure: mockInfrastructure
  }
}`,...(u=(i=t.parameters)==null?void 0:i.docs)==null?void 0:u.source}}};var d,m,l;n.parameters={...n.parameters,docs:{...(d=n.parameters)==null?void 0:d.docs,source:{originalSource:`{
  args: {
    infrastructure: [{
      name: 'clickhouse-cbt',
      status: 'running',
      type: 'clickhouse'
    }, {
      name: 'redis',
      status: 'stopped',
      type: 'redis'
    }]
  }
}`,...(l=(m=n.parameters)==null?void 0:m.docs)==null?void 0:l.source}}};var p,x,f;a.parameters={...a.parameters,docs:{...(p=a.parameters)==null?void 0:p.docs,source:{originalSource:`{
  args: {
    infrastructure: []
  }
}`,...(f=(x=a.parameters)==null?void 0:x.docs)==null?void 0:f.source}}};const N=["AllRunning","Mixed","Empty"];export{t as AllRunning,a as Empty,n as Mixed,N as __namedExportsOrder,j as default};
