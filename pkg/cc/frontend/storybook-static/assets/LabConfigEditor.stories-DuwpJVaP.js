import{j as m}from"./jsx-runtime-D_zvdyIk.js";import{l as S,h as o,d as c,H as i}from"./handlers-Nn_8ejmi.js";import{L as R}from"./LabConfigEditor-Dl3CMAAQ.js";import{g as t}from"./fixtures-CTg5x2hB.js";import"./iframe-CIsN2Fyl.js";import"./preload-helper-Dp1pzeXC.js";import"./Spinner-CVL8Mf24.js";const{fn:k}=__STORYBOOK_MODULE_TEST__,O={title:"Components/LabConfigEditor",component:R,decorators:[j=>m.jsx("div",{className:"bg-bg p-8",children:m.jsx(j,{})})],parameters:{msw:{handlers:S}},args:{onToast:k()}},a={},e={parameters:{msw:{handlers:[o.get("/api/config/lab",async()=>(await c(150),i.json({...t,mode:"hybrid",infrastructure:{...t.infrastructure,ClickHouse:{Xatu:{Mode:"external",ExternalURL:"clickhouse.example.com:9000"},CBT:{Mode:"local"}}}}))),...S.slice(1)]}}},r={parameters:{msw:{handlers:[o.get("/api/config/lab",async()=>(await c(999999),i.json(t)))]}}},n={parameters:{msw:{handlers:[o.get("/api/config/lab",async()=>(await c(100),new i(null,{status:500,statusText:"Internal Server Error"})))]}}},s={args:{onNavigateDashboard:k()}};var p,l,d;a.parameters={...a.parameters,docs:{...(p=a.parameters)==null?void 0:p.docs,source:{originalSource:"{}",...(d=(l=a.parameters)==null?void 0:l.docs)==null?void 0:d.source}}};var u,g,f;e.parameters={...e.parameters,docs:{...(u=e.parameters)==null?void 0:u.docs,source:{originalSource:`{
  parameters: {
    msw: {
      handlers: [http.get('/api/config/lab', async () => {
        await delay(150);
        return HttpResponse.json({
          ...mockLabConfig,
          mode: 'hybrid',
          infrastructure: {
            ...mockLabConfig.infrastructure,
            ClickHouse: {
              Xatu: {
                Mode: 'external',
                ExternalURL: 'clickhouse.example.com:9000'
              },
              CBT: {
                Mode: 'local'
              }
            }
          }
        });
      }), ...labConfigHandlers.slice(1)]
    }
  }
}`,...(f=(g=e.parameters)==null?void 0:g.docs)==null?void 0:f.source}}};var b,h,w;r.parameters={...r.parameters,docs:{...(b=r.parameters)==null?void 0:b.docs,source:{originalSource:`{
  parameters: {
    msw: {
      handlers: [http.get('/api/config/lab', async () => {
        await delay(999999);
        return HttpResponse.json(mockLabConfig);
      })]
    }
  }
}`,...(w=(h=r.parameters)==null?void 0:h.docs)==null?void 0:w.source}}};var x,y,C;n.parameters={...n.parameters,docs:{...(x=n.parameters)==null?void 0:x.docs,source:{originalSource:`{
  parameters: {
    msw: {
      handlers: [http.get('/api/config/lab', async () => {
        await delay(100);
        return new HttpResponse(null, {
          status: 500,
          statusText: 'Internal Server Error'
        });
      })]
    }
  }
}`,...(C=(y=n.parameters)==null?void 0:y.docs)==null?void 0:C.source}}};var E,L,H;s.parameters={...s.parameters,docs:{...(E=s.parameters)==null?void 0:E.docs,source:{originalSource:`{
  args: {
    onNavigateDashboard: fn()
  }
}`,...(H=(L=s.parameters)==null?void 0:L.docs)==null?void 0:H.source}}};const U=["Default","HybridMode","Loading","ErrorState","WithNavigateBack"];export{a as Default,n as ErrorState,e as HybridMode,r as Loading,s as WithNavigateBack,U as __namedExportsOrder,O as default};
